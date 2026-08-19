package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	internalErrors "github.com/rwx-cloud/rwx/internal/errors"
	"github.com/stretchr/testify/require"
)

func TestAPIClient_GetRetryOptions(t *testing.T) {
	t.Run("fetches the available choices for each target type", func(t *testing.T) {
		tests := []struct {
			name      string
			target    api.RetryTarget
			queryName string
		}{
			{name: "inferred", target: api.RetryTarget{ID: "target-123", Type: api.RetryTargetInferred}, queryName: "id"},
			{name: "run", target: api.RetryTarget{ID: "run-123", Type: api.RetryTargetRun}, queryName: "run_id"},
			{name: "task", target: api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask}, queryName: "task_id"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
					require.Equal(t, http.MethodGet, req.Method)
					require.Equal(t, "/mint/api/retries/options", req.URL.Path)
					require.Equal(t, tt.target.ID, req.URL.Query().Get(tt.queryName))
					require.Len(t, req.URL.Query(), 1)
					require.Equal(t, "application/json", req.Header.Get("Accept"))

					return &http.Response{
						Status:     "200 OK",
						StatusCode: http.StatusOK,
						Body: io.NopCloser(strings.NewReader(`{
							"retryable": true,
							"actions": [{"value":"standard","label":"Retry all tasks","description":"Every task will run again."}],
							"debug": {"supported":true,"placements":["end","start"],"default_placement":"end"},
							"tool_caches": [{"name":"bundler","scoped_task_keys":["test"],"usage_description":"Used by test"}]
						}`)),
					}, nil
				})

				options, err := client.GetRetryOptions(tt.target)

				require.NoError(t, err)
				require.True(t, options.Retryable)
				require.Equal(t, []api.RetryAction{{
					Value:       "standard",
					Label:       "Retry all tasks",
					Description: "Every task will run again.",
				}}, options.Actions)
				require.Equal(t, api.RetryDebugOptions{
					Supported:        true,
					Placements:       []string{"end", "start"},
					DefaultPlacement: "end",
				}, options.Debug)
				require.Equal(t, []api.RetryToolCache{{
					Name:             "bundler",
					ScopedTaskKeys:   []string{"test"},
					UsageDescription: "Used by test",
				}}, options.ToolCaches)
			})
		}
	})

	t.Run("requires a target identifier", func(t *testing.T) {
		client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			t.Fatal("the API must not be called")
			return nil, nil
		})

		_, err := client.GetRetryOptions(api.RetryTarget{Type: api.RetryTargetRun})

		require.EqualError(t, err, "missing retry target ID")
		require.ErrorIs(t, err, internalErrors.ErrBadRequest)
	})
}

func TestAPIClient_RequestRetry(t *testing.T) {
	t.Run("requests a task retry with all selected options", func(t *testing.T) {
		debug := true
		client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			require.Equal(t, http.MethodPost, req.Method)
			require.Equal(t, "/mint/api/retries", req.URL.Path)
			require.Equal(t, "task-123", req.URL.Query().Get("task_id"))
			require.Equal(t, "application/json", req.Header.Get("Content-Type"))
			require.Equal(t, "application/json", req.Header.Get("Accept"))

			var body map[string]any
			require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
			require.Equal(t, map[string]any{
				"retry": map[string]any{
					"action":           "no-tool-cache",
					"debug":            true,
					"debug_placement":  "start",
					"tool_cache_names": []any{"bundler", "golang"},
				},
			}, body)

			return &http.Response{
				Status:     "202 Accepted",
				StatusCode: http.StatusAccepted,
				Body:       io.NopCloser(strings.NewReader(`{"status":"retry_requested","run_id":"run-123","task_id":"task-123"}`)),
			}, nil
		})

		result, err := client.RequestRetry(api.RequestRetryConfig{
			Target:         api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
			Action:         "no-tool-cache",
			Debug:          &debug,
			DebugPlacement: "start",
			ToolCacheNames: []string{"bundler", "golang"},
		})

		require.NoError(t, err)
		require.Equal(t, api.RequestRetryResult{
			Status: "retry_requested",
			RunID:  "run-123",
			TaskID: "task-123",
		}, result)
	})

	t.Run("omits optional selections", func(t *testing.T) {
		client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			var body map[string]any
			require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
			require.Equal(t, map[string]any{
				"retry": map[string]any{"action": "standard"},
			}, body)

			return &http.Response{
				Status:     "202 Accepted",
				StatusCode: http.StatusAccepted,
				Body:       io.NopCloser(strings.NewReader(`{"status":"retry_requested","run_id":"run-123"}`)),
			}, nil
		})

		_, err := client.RequestRetry(api.RequestRetryConfig{
			Target: api.RetryTarget{ID: "run-123", Type: api.RetryTargetRun},
			Action: "standard",
		})

		require.NoError(t, err)
	})

	t.Run("returns validation details and refreshed options", func(t *testing.T) {
		client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				Status:     "422 Unprocessable Content",
				StatusCode: http.StatusUnprocessableEntity,
				Body: io.NopCloser(strings.NewReader(`{
					"error":"This retry configuration is not supported.",
					"errors":[{"field":"action","message":"Choose \u0060standard\u0060."}],
					"options":{"retryable":true,"actions":[{"value":"standard","label":"Standard retry"}],"debug":{"supported":false,"placements":[]},"tool_caches":[]}
				}`)),
			}, nil
		})

		_, err := client.RequestRetry(api.RequestRetryConfig{
			Target: api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
			Action: "clean",
		})

		var requestErr *api.RetryRequestError
		require.ErrorAs(t, err, &requestErr)
		require.EqualError(t, err, "This retry configuration is not supported.\n  action: Choose `standard`.")
		require.ErrorIs(t, err, internalErrors.ErrBadRequest)
		require.Equal(t, []api.RetryFieldError{{Field: "action", Message: "Choose `standard`."}}, requestErr.Errors)
		require.Equal(t, "standard", requestErr.Options.Actions[0].Value)
	})

	t.Run("requires a retry action", func(t *testing.T) {
		client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			t.Fatal("the API must not be called")
			return nil, nil
		})

		_, err := client.RequestRetry(api.RequestRetryConfig{
			Target: api.RetryTarget{ID: "run-123", Type: api.RetryTargetRun},
		})

		require.EqualError(t, err, "missing retry action")
		require.ErrorIs(t, err, internalErrors.ErrBadRequest)
	})
}
