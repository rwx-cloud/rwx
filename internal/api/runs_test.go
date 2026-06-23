package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/stretchr/testify/require"
)

func jsonBody(t *testing.T, v any) io.ReadCloser {
	t.Helper()
	encoded, err := json.Marshal(v)
	require.NoError(t, err)
	return io.NopCloser(strings.NewReader(string(encoded)))
}

func TestAPIClient_ListRuns(t *testing.T) {
	t.Run("maps single-valued filters onto scalar params", func(t *testing.T) {
		var captured *http.Request
		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			captured = req
			return &http.Response{
				StatusCode: 200,
				Body:       jsonBody(t, api.ListRunsResult{Runs: []api.RunSummary{}}),
			}, nil
		})

		_, err := c.ListRuns(api.ListRunsConfig{
			RepositoryNames: []string{"rwx-cloud/cloud"},
			Branches:        []string{"main"},
			CommitShas:      []string{"abc123"},
			DefinitionPaths: []string{".rwx/ci.yml"},
			MyRuns:          true,
			Limit:           25,
			Cursor:          "cursor-token",
		})
		require.NoError(t, err)

		require.Equal(t, "/mint/api/runs", captured.URL.Path)
		query := captured.URL.Query()
		require.Equal(t, "rwx-cloud/cloud", query.Get("repository_name"))
		require.Equal(t, "main", query.Get("branch"))
		require.Equal(t, "abc123", query.Get("commit_sha"))
		require.Equal(t, ".rwx/ci.yml", query.Get("definition_path"))
		require.Equal(t, "true", query.Get("my_runs"))
		require.Equal(t, "25", query.Get("limit"))
		require.Equal(t, "cursor-token", query.Get("cursor"))
	})

	t.Run("uses the scalar param for a lone filter value", func(t *testing.T) {
		var captured *http.Request
		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			captured = req
			return &http.Response{StatusCode: 200, Body: jsonBody(t, api.ListRunsResult{})}, nil
		})

		_, err := c.ListRuns(api.ListRunsConfig{
			Branches:       []string{"main"},
			ResultStatuses: []string{"succeeded"},
		})
		require.NoError(t, err)

		query := captured.URL.Query()
		require.Equal(t, "main", query.Get("branch"))
		require.Equal(t, "succeeded", query.Get("result_status"))
		require.Empty(t, query["branch_names[]"])
		require.Empty(t, query["result_statuses[]"])
	})

	t.Run("uses the plural param when a filter has multiple values", func(t *testing.T) {
		var captured *http.Request
		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			captured = req
			return &http.Response{StatusCode: 200, Body: jsonBody(t, api.ListRunsResult{})}, nil
		})

		_, err := c.ListRuns(api.ListRunsConfig{
			Branches:       []string{"main", "develop"},
			ResultStatuses: []string{"succeeded", "failed"},
		})
		require.NoError(t, err)

		query := captured.URL.Query()
		require.Equal(t, []string{"main", "develop"}, query["branch_names[]"])
		require.Empty(t, query.Get("branch"))
		require.Equal(t, []string{"succeeded", "failed"}, query["result_statuses[]"])
		require.Empty(t, query.Get("result_status"))
	})

	t.Run("decodes runs, nested status, and pagination", func(t *testing.T) {
		next := "next-page-cursor"
		tag := "v1.2.3"
		body := api.ListRunsResult{
			Runs: []api.RunSummary{
				{
					ID:             "run-1",
					Status:         api.RunStatus{Result: "succeeded", Execution: "finished", FinishedSubStatus: "not_applicable"},
					RepositoryName: "rwx-cloud/cloud",
					Branch:         "main",
					Tag:            &tag,
					CommitSha:      "abcdef1234567890",
					DefinitionPath: ".rwx/ci.yml",
					Trigger:        "github",
					Title:          "CI",
					RunURL:         "https://cloud.rwx.com/runs/run-1",
				},
			},
			Pagination: api.ListRunsPagination{NextCursor: &next, Limit: 50},
		}
		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: jsonBody(t, body)}, nil
		})

		result, err := c.ListRuns(api.ListRunsConfig{})
		require.NoError(t, err)
		require.Len(t, result.Runs, 1)
		require.Equal(t, "run-1", result.Runs[0].ID)
		require.Equal(t, "succeeded", result.Runs[0].Status.Result)
		require.Equal(t, "finished", result.Runs[0].Status.Execution)
		require.NotNil(t, result.Pagination.NextCursor)
		require.Equal(t, "next-page-cursor", *result.Pagination.NextCursor)
		require.Equal(t, 50, result.Pagination.Limit)
	})

	t.Run("represents the final page with a nil next cursor", func(t *testing.T) {
		raw := `{"runs":[],"pagination":{"next_cursor":null,"limit":50}}`
		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(raw))}, nil
		})

		result, err := c.ListRuns(api.ListRunsConfig{})
		require.NoError(t, err)
		require.Nil(t, result.Pagination.NextCursor)
		require.Equal(t, 50, result.Pagination.Limit)
	})

	t.Run("surfaces a 200 suggestions envelope without erroring", func(t *testing.T) {
		raw := `{"runs":[{"id":"run-1","status":{"result":"succeeded"}}],` +
			`"pagination":{"next_cursor":null,"limit":50},` +
			`"suggestions":{"branch_names":[{"value":"develp","suggestions":["develop"]}]}}`
		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(raw))}, nil
		})

		result, err := c.ListRuns(api.ListRunsConfig{})
		require.NoError(t, err)
		// Suggestions can ride along with a non-empty runs list.
		require.Len(t, result.Runs, 1)
		require.Len(t, result.Suggestions["branch_names"], 1)
		require.Equal(t, "develp", result.Suggestions["branch_names"][0].Value)
		require.Equal(t, []string{"develop"}, result.Suggestions["branch_names"][0].Suggestions)
	})

	t.Run("parses the structured 400 invalid-status body into a typed error", func(t *testing.T) {
		raw := `{"errors":[{"key":"result_status","provided_value":"succeded",` +
			`"suggested_value":"succeeded","valid_values":["succeeded","debugged","sandboxed","failed","no_result"]}]}`
		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			return &http.Response{Status: "400 Bad Request", StatusCode: 400, Body: io.NopCloser(strings.NewReader(raw))}, nil
		})

		_, err := c.ListRuns(api.ListRunsConfig{ResultStatuses: []string{"succeded"}})
		require.Error(t, err)

		var filterErr *api.InvalidRunFilterError
		require.True(t, errors.As(err, &filterErr))
		require.Len(t, filterErr.Entries, 1)
		require.Equal(t, "result_status", filterErr.Entries[0].Key)
		require.Equal(t, "succeded", filterErr.Entries[0].ProvidedValue)
		require.NotNil(t, filterErr.Entries[0].SuggestedValue)
		require.Equal(t, "succeeded", *filterErr.Entries[0].SuggestedValue)
		require.Contains(t, err.Error(), "did you mean")
		require.True(t, errors.Is(err, errors.ErrBadRequest))
	})

	t.Run("falls back to valid_values when the 400 suggestion is null", func(t *testing.T) {
		raw := `{"errors":[{"key":"execution_status","provided_value":"zzz",` +
			`"suggested_value":null,"valid_values":["waiting","in_progress","finished","aborted"]}]}`
		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			return &http.Response{Status: "400 Bad Request", StatusCode: 400, Body: io.NopCloser(strings.NewReader(raw))}, nil
		})

		_, err := c.ListRuns(api.ListRunsConfig{})
		require.Error(t, err)
		var filterErr *api.InvalidRunFilterError
		require.True(t, errors.As(err, &filterErr))
		require.Nil(t, filterErr.Entries[0].SuggestedValue)
		require.Contains(t, err.Error(), "valid:")
		require.Contains(t, err.Error(), "waiting")
	})

	t.Run("handles the malformed-cursor 400 body", func(t *testing.T) {
		raw := `{"error":"Invalid cursor"}`
		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			return &http.Response{Status: "400 Bad Request", StatusCode: 400, Body: io.NopCloser(strings.NewReader(raw))}, nil
		})

		_, err := c.ListRuns(api.ListRunsConfig{Cursor: "bogus"})
		require.Error(t, err)
		var filterErr *api.InvalidRunFilterError
		require.False(t, errors.As(err, &filterErr), "a bad cursor is not a filter validation error")
		require.True(t, errors.Is(err, errors.ErrBadRequest))
		require.Contains(t, err.Error(), "Invalid cursor")
	})

	t.Run("sends status filters as-is, leaving enum validation to the server", func(t *testing.T) {
		var captured *http.Request
		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			captured = req
			return &http.Response{StatusCode: 200, Body: jsonBody(t, api.ListRunsResult{})}, nil
		})

		// An unknown value is not rejected client-side; it reaches the server, which
		// owns the enum and answers with a structured 400 (exercised above).
		_, err := c.ListRuns(api.ListRunsConfig{ResultStatuses: []string{"succeded"}})
		require.NoError(t, err)
		require.Equal(t, "succeded", captured.URL.Query().Get("result_status"))
	})

	t.Run("synthesizes an actionable message from a 429 with Retry-After", func(t *testing.T) {
		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			header := http.Header{}
			header.Set("Retry-After", "60")
			return &http.Response{
				Status:     "429 Too Many Requests",
				StatusCode: 429,
				Header:     header,
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		})

		_, err := c.ListRuns(api.ListRunsConfig{})
		require.Error(t, err)
		var rateErr *api.RateLimitedError
		require.True(t, errors.As(err, &rateErr))
		require.Equal(t, 60, rateErr.RetryAfterSeconds)
		require.Contains(t, err.Error(), "60 seconds")
	})
}

// TestRunStatus_StatusHashShape pins the shared RunStatus to the status_hash shape
// emitted by both the runs index `status` and results details, and guards against
// regressing onto the divergent `runs#show` `run_status` shape.
func TestRunStatus_StatusHashShape(t *testing.T) {
	t.Run("deserializes the status_hash keys", func(t *testing.T) {
		raw := `{
			"result": "succeeded",
			"execution": "finished",
			"waiting_sub_status": "not_applicable",
			"aborted_sub_status": "not_applicable",
			"finished_sub_status": "not_applicable"
		}`
		var status api.RunStatus
		require.NoError(t, json.Unmarshal([]byte(raw), &status))
		require.Equal(t, "succeeded", status.Result)
		require.Equal(t, "finished", status.Execution)
		require.Equal(t, "not_applicable", status.WaitingSubStatus)
		require.Equal(t, "not_applicable", status.AbortedSubStatus)
		require.Equal(t, "not_applicable", status.FinishedSubStatus)
	})

	t.Run("does not absorb the divergent runs#show run_status keys", func(t *testing.T) {
		// `runs#show` uses *_status (not *_sub_status) plus extra fields; none of
		// those should populate the shared status_hash type.
		raw := `{
			"result": "succeeded",
			"execution": "finished",
			"predicted_result": "succeeded",
			"waiting_status": "leaked",
			"aborted_status": "leaked",
			"finished_status": "leaked",
			"cancelation_requested?": false
		}`
		var status api.RunStatus
		require.NoError(t, json.Unmarshal([]byte(raw), &status))
		require.Equal(t, "succeeded", status.Result)
		require.Equal(t, "finished", status.Execution)
		require.Empty(t, status.WaitingSubStatus)
		require.Empty(t, status.AbortedSubStatus)
		require.Empty(t, status.FinishedSubStatus)
	})
}
