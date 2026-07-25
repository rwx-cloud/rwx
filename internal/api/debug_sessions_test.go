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

func TestAPIClient_AttachDebugSession(t *testing.T) {
	t.Run("attaches a named session", func(t *testing.T) {
		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			require.Equal(t, http.MethodPost, req.Method)
			require.Equal(t, "/mint/api/tasks/task-123/debug_sessions", req.URL.Path)
			require.Equal(t, "application/json", req.Header.Get("Content-Type"))

			var body map[string]any
			require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
			require.Equal(t, map[string]any{"name": "shell"}, body)

			return &http.Response{
				Status:     "202 Accepted",
				StatusCode: http.StatusAccepted,
				Body: io.NopCloser(strings.NewReader(`{
					"debug_session": {
						"id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
						"name": "shell",
						"status": "starting"
					}
				}`)),
			}, nil
		})

		session, err := c.AttachDebugSession(api.AttachDebugSessionConfig{
			TaskID: "task-123",
			Name:   "shell",
		})

		require.NoError(t, err)
		require.Equal(t, api.DebugSessionSummary{
			ID:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Name:   "shell",
			Status: "starting",
		}, session)
	})

	t.Run("requires a task ID", func(t *testing.T) {
		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			t.Fatal("should not make a request")
			return nil, nil
		})

		_, err := c.AttachDebugSession(api.AttachDebugSessionConfig{})

		require.EqualError(t, err, "missing task ID")
		require.ErrorIs(t, err, internalErrors.ErrBadRequest)
	})

	t.Run("maps admission errors", func(t *testing.T) {
		tests := []struct {
			name       string
			status     string
			statusCode int
			code       string
			sentinel   error
			message    string
		}{
			{
				name:       "invalid task type",
				status:     "400 Bad Request",
				statusCode: http.StatusBadRequest,
				code:       "invalid_task",
				sentinel:   internalErrors.ErrBadRequest,
				message:    `task "task-123" cannot host an attached SSH session`,
			},
			{
				name:       "locked vault",
				status:     "403 Forbidden",
				statusCode: http.StatusForbidden,
				code:       "vault_access_required",
				sentinel:   internalErrors.ErrUnauthenticated,
				message:    "the access token cannot unlock every vault used by this task",
			},
			{
				name:       "task not found",
				status:     "404 Not Found",
				statusCode: http.StatusNotFound,
				code:       "task_not_found",
				sentinel:   internalErrors.ErrNotFound,
				message:    `task "task-123" was not found`,
			},
			{
				name:       "unavailable task server",
				status:     "410 Gone",
				statusCode: http.StatusGone,
				code:       "task_not_running",
				sentinel:   internalErrors.ErrGone,
				message:    `task "task-123" is no longer running`,
			},
			{
				name:       "duplicate name",
				status:     "422 Unprocessable Entity",
				statusCode: http.StatusUnprocessableEntity,
				code:       "duplicate_open_name",
				sentinel:   internalErrors.ErrBadRequest,
				message:    `an open debug session named "shell" already exists`,
			},
			{
				name:       "task stopped during admission",
				status:     "422 Unprocessable Entity",
				statusCode: http.StatusUnprocessableEntity,
				code:       "task_not_running",
				sentinel:   internalErrors.ErrGone,
				message:    `task "task-123" is no longer running`,
			},
			{
				name:       "attachment closed",
				status:     "422 Unprocessable Entity",
				statusCode: http.StatusUnprocessableEntity,
				code:       "attachment_closed",
				sentinel:   internalErrors.ErrGone,
				message:    `task "task-123" no longer accepts attached SSH sessions`,
			},
			{
				name:       "task server error",
				status:     "502 Bad Gateway",
				statusCode: http.StatusBadGateway,
				code:       "task_server_error",
				sentinel:   internalErrors.ErrInternalServerError,
				message:    "unable to attach an SSH session through the task server",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
					body := `{}`
					if tt.code != "task_not_found" {
						bodyBytes, err := json.Marshal(map[string]string{"error": tt.code})
						require.NoError(t, err)
						body = string(bodyBytes)
					}
					return &http.Response{
						Status:     tt.status,
						StatusCode: tt.statusCode,
						Body:       io.NopCloser(strings.NewReader(body)),
					}, nil
				})

				_, err := c.AttachDebugSession(api.AttachDebugSessionConfig{
					TaskID: "task-123",
					Name:   "shell",
				})
				var attachmentErr *api.DebugSessionAttachmentError
				require.ErrorAs(t, err, &attachmentErr)
				require.Equal(t, tt.code, attachmentErr.Code)
				require.EqualError(t, err, tt.message)
				require.ErrorIs(t, err, tt.sentinel)
			})
		}
	})
}
