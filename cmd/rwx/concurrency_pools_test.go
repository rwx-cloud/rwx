package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestConcurrencyPoolsCommands(t *testing.T) {
	require.Same(t, concurrencyPoolsCmd, findSubcommand(rootCmd, "concurrency-pools"))
	require.False(t, concurrencyPoolsCmd.Runnable())
	require.Len(t, concurrencyPoolsCmd.Commands(), 2)

	list := findSubcommand(concurrencyPoolsCmd, "list")
	require.Contains(t, list.Aliases, "ls")
	for _, flag := range []string{"search", "limit", "cursor"} {
		require.NotNil(t, list.Flags().Lookup(flag))
	}
	require.NoError(t, list.Args(list, nil))
	require.Error(t, list.Args(list, []string{"pool-1"}))

	show := findSubcommand(concurrencyPoolsCmd, "show")
	require.Contains(t, show.Aliases, "get")
	for _, flag := range []string{"limit", "cursor"} {
		require.NotNil(t, show.Flags().Lookup(flag))
	}
	require.Error(t, show.Args(show, nil))
	require.NoError(t, show.Args(show, []string{"pool-1"}))
	require.Error(t, show.Args(show, []string{"pool-1", "pool-2"}))
}

func TestConcurrencyPoolsBarePrintsHelp(t *testing.T) {
	out, err := executeRoot(t, "concurrency-pools")
	require.NoError(t, err)
	require.Contains(t, out, "Available Commands")
	require.Contains(t, out, "list")
	require.Contains(t, out, "show")
}

func TestConcurrencyPoolsFlagsReachAPI(t *testing.T) {
	originalService, originalFormat := service, Format
	t.Cleanup(func() {
		service, Format = originalService, originalFormat
	})
	Format = "text"

	var requests []*http.Request
	client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, req)
		body := `{"concurrency_pools":[],"pagination":{"next_cursor":null,"limit":3}}`
		if strings.HasSuffix(req.URL.Path, "/queue") {
			body = `{"leases":[],"pagination":{"next_cursor":null,"limit":4}}`
		} else if req.URL.Path != "/mint/api/concurrency_pools" {
			body = `{"id":"pool/one","status":"idle","queue_size":0,"usage_count_last_7_days":0,"latest_configuration":null,"recent_runs":[]}`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	service = cli.Service{Config: cli.Config{APIClient: client}}

	list := newConcurrencyPoolsListCommand()
	list.SetArgs([]string{"--search", "pool one", "--limit", "3", "--cursor", "list-cursor"})
	require.NoError(t, list.Execute())
	require.Equal(t, "pool one", requests[0].URL.Query().Get("search"))
	require.Equal(t, "3", requests[0].URL.Query().Get("limit"))
	require.Equal(t, "list-cursor", requests[0].URL.Query().Get("cursor"))

	show := newConcurrencyPoolsShowCommand()
	show.SetArgs([]string{"pool/one", "--limit", "4", "--cursor", "queue-cursor"})
	require.NoError(t, show.Execute())
	require.Len(t, requests, 3)
	require.Equal(t, "/mint/api/concurrency_pools/pool%2Fone", requests[1].URL.EscapedPath())
	require.Equal(t, "/mint/api/concurrency_pools/pool%2Fone/queue", requests[2].URL.EscapedPath())
	require.Equal(t, "4", requests[2].URL.Query().Get("limit"))
	require.Equal(t, "queue-cursor", requests[2].URL.Query().Get("cursor"))
}

func TestRenderConcurrencyPoolsTable(t *testing.T) {
	lastUsed := "2026-10-05T18:00:00Z"
	var buf bytes.Buffer
	renderConcurrencyPoolsTable(&buf, []api.ConcurrencyPool{
		{
			ID:                  "pool-1",
			Status:              "active",
			QueueSize:           3,
			UsageCountLast7Days: 42,
			LastRequestedAt:     &lastUsed,
			LatestConfiguration: &api.ConcurrencyPoolConfiguration{Capacity: 2, OnOverflow: "queue"},
		},
		{ID: "pool-without-configuration", Status: "idle"},
	})

	out := buf.String()
	require.Contains(t, out, "POOL")
	require.Contains(t, out, "STATUS")
	require.Contains(t, out, "QUEUE")
	require.Contains(t, out, "CAPACITY")
	require.Contains(t, out, "ON OVERFLOW")
	require.Contains(t, out, "USES (7D)")
	require.Contains(t, out, "LAST USED")
	require.Contains(t, out, "pool-1")
	require.Contains(t, out, "active")
	require.Contains(t, out, "42")
	require.Contains(t, out, "2026-10-05T18:00:00Z")
	require.Contains(t, out, "pool-without-configuration")
}

func TestRenderConcurrencyPoolIncludesQueueAndRecentRuns(t *testing.T) {
	resultStatus := "failed"
	repository := "rwx-cloud/rwx"
	branch := "main"
	title := "CI"
	started := "2026-10-05T17:00:00Z"
	completed := "2026-10-05T17:01:00Z"
	var buf bytes.Buffer
	renderConcurrencyPool(&buf, &cli.ShowConcurrencyPoolResult{
		ConcurrencyPool: api.ConcurrencyPool{
			ID:                  "pool-1",
			Status:              "paused",
			QueueSize:           1,
			LatestConfiguration: &api.ConcurrencyPoolConfiguration{Capacity: 1, OnOverflow: "cancel-waiting"},
			RecentRuns: []api.ConcurrencyPoolRun{{
				ID:             "run-recent",
				Status:         api.ConcurrencyPoolRunStatus{Result: &resultStatus, Execution: "finished"},
				RepositoryName: &repository,
				Branch:         &branch,
				Title:          &title,
				StartedAt:      &started,
				CompletedAt:    &completed,
			}},
		},
		Queue: []api.ConcurrencyPoolLease{{
			Position:    4,
			RunID:       "run-queued",
			State:       "requested",
			Capacity:    1,
			OnOverflow:  "queue",
			RequestedAt: "2026-10-05T18:00:00Z",
		}},
	})

	out := buf.String()
	require.Contains(t, out, "Pool: pool-1")
	require.Contains(t, out, "Capacity: 1")
	require.Contains(t, out, "CURRENT QUEUE")
	require.Contains(t, out, "run-queued")
	require.Contains(t, out, "RECENT RUNS")
	require.Contains(t, out, "run-recent")
	require.Contains(t, out, "failed")
}

func TestConcurrencyPoolsJSONUsesPascalCaseAndPreservesPagination(t *testing.T) {
	next := "next-cursor"
	var buf bytes.Buffer
	err := printConcurrencyPoolsJSON(&buf, &api.ListConcurrencyPoolsResult{
		ConcurrencyPools: []api.ConcurrencyPool{{
			ID:                  "pool-1",
			LatestConfiguration: &api.ConcurrencyPoolConfiguration{Capacity: 2, OnOverflow: "queue"},
		}},
		Pagination: api.ConcurrencyPoolPagination{NextCursor: &next, Limit: 10},
	})
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	pools := decoded["ConcurrencyPools"].([]any)
	pool := pools[0].(map[string]any)
	require.Equal(t, "pool-1", pool["ID"])
	configuration := pool["LatestConfiguration"].(map[string]any)
	require.Equal(t, float64(2), configuration["Capacity"])
	require.Equal(t, "queue", configuration["OnOverflow"])
	pagination := decoded["Pagination"].(map[string]any)
	require.Equal(t, "next-cursor", pagination["NextCursor"])
	require.Equal(t, float64(10), pagination["Limit"])
}

func TestConcurrencyPoolsPaginationHint(t *testing.T) {
	next := "next-cursor"
	var buf bytes.Buffer
	printConcurrencyPoolsPaginationHint(&buf, api.ConcurrencyPoolPagination{NextCursor: &next})
	require.Contains(t, buf.String(), "--cursor next-cursor")

	buf.Reset()
	printConcurrencyPoolsPaginationHint(&buf, api.ConcurrencyPoolPagination{})
	require.Empty(t, buf.String())
}
