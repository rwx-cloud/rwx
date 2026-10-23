package api_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/stretchr/testify/require"
)

func TestCompactDetails(t *testing.T) {
	body, err := os.ReadFile("testdata/compact_results.json")
	require.NoError(t, err)
	var fixture map[string][]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &fixture))
	for _, taskKey := range []string{"", "checks"} {
		t.Run("task key="+taskKey, func(t *testing.T) {
			pages := map[string]int{}
			var requests []string
			c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, "compact", req.URL.Query().Get("view"))
				require.Equal(t, "100", req.URL.Query().Get("limit"))
				id := strings.TrimPrefix(req.URL.Path, "/mint/api/results/")
				expectedKey := ""
				if id == "run" {
					expectedKey = taskKey
					if taskKey != "" {
						id = "group-old"
					}
				}
				require.Equal(t, expectedKey, req.URL.Query().Get("task_key"))
				index := pages[id]
				require.Less(t, index, len(fixture[id]))
				if index > 0 {
					var previous struct {
						Next string `json:"next_cursor"`
					}
					require.NoError(t, json.Unmarshal(fixture[id][index-1], &previous))
					require.Equal(t, previous.Next, req.URL.Query().Get("cursor"))
				} else {
					require.Empty(t, req.URL.Query().Get("cursor"))
				}
				pages[id]++
				requests = append(requests, id)
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(fixture[id][index])))}, nil
			})
			result, err := c.GetRunDetails(api.RunDetailsConfig{RunID: "run", TaskKey: taskKey, Compact: true})
			require.NoError(t, err)
			group := result
			if taskKey == "" {
				require.Equal(t, []string{"run", "run", "run", "group-old", "group-old", "leaf-old", "a-release"}, requests)
				require.Contains(t, result, "repository_name")
				require.Nil(t, result["repository_name"])
				require.Equal(t, []any{map[string]any{"type": "warning", "text": "run warning"}}, result["messages"])
				tasks := result["tasks"].([]any)
				require.Len(t, tasks, 2)
				group = tasks[0].(map[string]any)
				require.Equal(t, "a-release", tasks[1].(map[string]any)["id"])
			} else {
				require.Equal(t, []string{"group-old", "group-old", "leaf-old"}, requests)
			}
			require.Equal(t, float64(2), group["attempt_number"])
			require.Equal(t, float64(7), group["completed_runtime_seconds"])
			require.Equal(t, map[string]any{"completed": false, "backoff_ms": float64(2000)}, group["polling"])
			require.Equal(t, []any{
				map[string]any{"key": "first", "value": "before"},
				map[string]any{"key": "雪", "value": "山\n\"終"},
				map[string]any{"key": "last", "value": "after"},
			}, group["outputs"].(map[string]any)["values"])
			require.Equal(t, []any{"build"}, group["dependencies"])
			require.Equal(t, []any{"ci.build"}, group["scoped_dependencies"])
			require.Equal(t, "leaf-old", group["subtasks"].([]any)[0].(map[string]any)["id"])
			require.Equal(t, float64(1), group["subtasks"].([]any)[0].(map[string]any)["attempt_number"])
			for _, key := range []string{"fragments", "truncated", "next_cursor", "env", "metrics"} {
				require.NotContains(t, group, key)
			}
		})
	}
}

func TestCompactCursorRestart(t *testing.T) {
	for _, code := range []string{"stale_cursor", "invalid_cursor"} {
		t.Run(code, func(t *testing.T) {
			calls := 0
			c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				calls++
				status := 200
				body := `{"id":"task","subtasks":[],"messages":[],"fragments":[],"truncated":false,"next_cursor":null}`
				switch calls {
				case 1:
					body = `{"id":"task","subtasks":[],"messages":[{"text":"discard me"}],"fragments":[{"collection":"values","position":0,"offset":0,"json":"{","complete":false}],"truncated":true,"next_cursor":"old"}`
				case 2:
					require.Equal(t, "old", req.URL.Query().Get("cursor"))
					status = 409
					if code == "invalid_cursor" {
						status = 400
					}
					body = `{"error_code":"` + code + `"}`
				case 3:
					require.Empty(t, req.URL.Query().Get("cursor"))
				default:
					t.Fatal("unbounded cursor retries")
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			result, err := c.GetRunDetails(api.RunDetailsConfig{RunID: "task", Compact: true})
			require.NoError(t, err)
			require.Equal(t, 3, calls)
			require.Empty(t, result["messages"])
			require.Empty(t, result["outputs"].(map[string]any)["values"])
		})
	}
}

func TestCompactDetailsRejectsIncompleteResults(t *testing.T) {
	for _, tc := range []struct {
		name, body    string
		status, calls int
		message       string
	}{
		{"unsupported", `{"id":"task","env":{"SECRET":"legacy"}}`, 200, 1, "does not support"},
		{"repeated cursor", `{"id":"task","truncated":true,"next_cursor":"same"}`, 200, 2, "continuation"},
		{"missing cursor", `{"id":"task","truncated":true}`, 200, 1, "continuation"},
		{"partial", `{"id":"task","truncated":false,"fragments":[{"collection":"dependencies","position":0,"offset":0,"json":"\"雪","complete":false}]}`, 200, 1, "incomplete"},
		{"offset", `{"id":"task","truncated":false,"fragments":[{"collection":"dependencies","position":0,"offset":2,"json":"\"x\"","complete":true}]}`, 200, 1, "offset"},
		{"stale", `{"error_code":"stale_cursor"}`, 409, 3, "kept changing"},
		{"invalid", `{"error_code":"invalid_cursor"}`, 400, 3, "kept changing"},
		{"unrelated bad request", `{"error":"bad request"}`, 400, 1, "bad request"},
		{"unrelated conflict", `{"error":"other conflict"}`, 409, 1, "other conflict"},
		{"oversize", `{"error_code":"too_large"}`, 413, 1, "too_large"},
		{"cycle", `{"id":"task","subtasks":[{"id":"task"}],"truncated":false}`, 200, 1, "cycle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := api.NewClientWithRoundTrip(func(*http.Request) (*http.Response, error) {
				calls++
				require.LessOrEqual(t, calls, tc.calls)
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			result, err := c.GetRunDetails(api.RunDetailsConfig{RunID: "task", Compact: true})
			require.Nil(t, result)
			require.ErrorContains(t, err, tc.message)
			require.Equal(t, tc.calls, calls)
		})
	}
}

func TestCompactRestartDiscardsParentTraversal(t *testing.T) {
	calls := 0
	c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
		calls++
		status := 200
		body := ""
		switch calls {
		case 1:
			require.Equal(t, "/mint/api/results/run", req.URL.Path)
			body = `{"id":"run","tasks":[{"id":"old"}],"messages":[{"text":"discard parent too"}],"truncated":false}`
		case 2:
			require.Equal(t, "/mint/api/results/old", req.URL.Path)
			status, body = 409, `{"error_code":"stale_cursor"}`
		case 3:
			require.Equal(t, "/mint/api/results/run", req.URL.Path)
			require.Empty(t, req.URL.Query().Get("cursor"))
			body = `{"id":"run","tasks":[{"id":"new"}],"messages":[],"truncated":false}`
		case 4:
			require.Equal(t, "/mint/api/results/new", req.URL.Path)
			body = `{"id":"new","subtasks":[],"truncated":false}`
		default:
			t.Fatal("unexpected request")
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	result, err := c.GetRunDetails(api.RunDetailsConfig{RunID: "run", Compact: true})
	require.NoError(t, err)
	require.Equal(t, 4, calls)
	require.Empty(t, result["messages"])
	require.Equal(t, "new", result["tasks"].([]any)[0].(map[string]any)["id"])
}

func TestCompactTraversalLimit(t *testing.T) {
	calls := 0
	c := api.NewClientWithRoundTrip(func(*http.Request) (*http.Response, error) {
		calls++
		require.LessOrEqual(t, calls, 10000)
		body := fmt.Sprintf(`{"id":"run","tasks":[],"truncated":true,"next_cursor":"cursor-%d"}`, calls)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	result, err := c.GetRunDetails(api.RunDetailsConfig{RunID: "run", Compact: true})
	require.Nil(t, result)
	require.ErrorContains(t, err, "10000 request traversal limit")
	require.Equal(t, 10000, calls)
}

func TestCompactFragmentCollections(t *testing.T) {
	for _, tc := range []struct {
		collection, prefix, suffix string
		expected                   any
	}{
		{"messages", `{"`, `type":"warning","text":"雪山"}`, map[string]any{"type": "warning", "text": "雪山"}},
		{"links", `{"`, `name":"雪山","url":"https://rwx.com"}`, map[string]any{"name": "雪山", "url": "https://rwx.com"}},
		{"dependencies", `"雪`, `山"`, "雪山"},
		{"scoped_dependencies", `"雪`, `.山"`, "雪.山"},
	} {
		t.Run(tc.collection, func(t *testing.T) {
			calls := 0
			c := api.NewClientWithRoundTrip(func(*http.Request) (*http.Response, error) {
				calls++
				require.LessOrEqual(t, calls, 2)
				segment, offset, next := tc.prefix, 0, "next"
				if calls == 2 {
					segment, offset, next = tc.suffix, 2, ""
				}
				page := map[string]any{
					"id": "task", "subtasks": []any{}, "messages": []any{}, "dependencies": []any{}, "scoped_dependencies": []any{},
					"outputs":   map[string]any{"values": []any{}, "links": []any{}},
					"truncated": calls == 1, "next_cursor": next,
					"fragments": []any{map[string]any{"collection": tc.collection, "position": 0, "offset": offset, "json": segment, "complete": calls == 2}},
				}
				body, err := json.Marshal(page)
				require.NoError(t, err)
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
			})
			result, err := c.GetRunDetails(api.RunDetailsConfig{RunID: "task", Compact: true})
			require.NoError(t, err)
			if tc.collection == "links" {
				result = result["outputs"].(map[string]any)
			}
			require.Equal(t, []any{tc.expected}, result[tc.collection])
		})
	}
}
