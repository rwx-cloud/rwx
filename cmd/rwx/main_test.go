package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/stretchr/testify/require"
)

func TestClassifyError(t *testing.T) {
	t.Run("returns unknown for nil and bare errors", func(t *testing.T) {
		require.Equal(t, "unknown", classifyError(errors.New("something went wrong")))
	})

	cases := []struct {
		name     string
		err      error
		expected string
	}{
		{"bad_request", errors.ErrBadRequest, "bad_request"},
		{"unauthenticated", errors.ErrUnauthenticated, "unauthenticated"},
		{"not_found", errors.ErrNotFound, "not_found"},
		{"file_not_found", errors.ErrFileNotExists, "file_not_found"},
		{"gone", errors.ErrGone, "gone"},
		{"internal_server_error", errors.ErrInternalServerError, "internal_server_error"},
		{"ssh_failed", errors.ErrSSH, "ssh_failed"},
		{"patch_failed", errors.ErrPatch, "patch_failed"},
		{"timeout", errors.ErrTimeout, "timeout"},
		{"lsp_error", errors.ErrLSP, "lsp_error"},
		{"ambiguous_task_key", errors.ErrAmbiguousTaskKey, "ambiguous_task_key"},
		{"ambiguous_definition_path", errors.ErrAmbiguousDefinitionPath, "ambiguous_definition_path"},
		{"network_transient_error", errors.ErrNetworkTransient, "network_transient_error"},
		{"sandbox_setup_failure", errors.ErrSandboxSetupFailure, "sandbox_setup_failure"},
		{"sandbox_no_git_dir", errors.ErrSandboxNoGitDir, "sandbox_no_git_dir"},
	}

	for _, tc := range cases {
		t.Run(tc.name+"/bare", func(t *testing.T) {
			require.Equal(t, tc.expected, classifyError(tc.err))
		})

		t.Run(tc.name+"/wrapped", func(t *testing.T) {
			wrapped := errors.WrapSentinel(fmt.Errorf("some context: inner"), tc.err)
			require.Equal(t, tc.expected, classifyError(wrapped))
		})
	}

	t.Run("file_not_found matches os.ErrNotExist wrappers", func(t *testing.T) {
		// os.Open of a missing file returns a *PathError that Is(os.ErrNotExist)
		_, err := os.Open("/this/path/definitely/does/not/exist/rwx-684")
		require.Error(t, err)
		require.Equal(t, "file_not_found", classifyError(err))
	})
}

func TestScrubErrorMessage(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	require.NotEmpty(t, home)

	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "plain message passes through",
			input:    "failed to open task.yaml",
			expected: "failed to open task.yaml",
		},
		{
			name:     "replaces home directory with tilde",
			input:    "failed to open " + home + "/project/.rwx.yaml",
			expected: "failed to open ~/project/.rwx.yaml",
		},
		{
			name:     "redacts URL userinfo",
			input:    "clone failed at https://user:secret@github.com/rwx/repo",
			expected: "clone failed at https://<redacted>@github.com/rwx/repo",
		},
		{
			name:     "redacts JWT-shaped token",
			input:    "bad auth: eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
			expected: "bad auth: <redacted>",
		},
		{
			name:     "does not redact short dotted identifiers",
			input:    "parse error at com.example.Foo",
			expected: "parse error at com.example.Foo",
		},
		{
			name:     "redacts long hex-like token",
			input:    "invalid token abcdef0123456789abcdef0123456789abcdef01",
			expected: "invalid token <redacted>",
		},
		{
			name:     "redacts UUID-shaped run",
			input:    "run 550e8400e29b41d4a716446655440000 not found",
			expected: "run <redacted> not found",
		},
		{
			name:     "truncates very long messages",
			input:    strings.Repeat("foo bar ", 40),
			expected: strings.Repeat("foo bar ", 40)[:errorMessageMaxRunes] + "...",
		},
		{
			name:     "composite: home path + token",
			input:    "cache miss at " + home + "/.rwx/cache/abcdef0123456789abcdef0123456789abcdef.json",
			expected: "cache miss at ~/.rwx/cache/<redacted>.json",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, scrubErrorMessage(tc.input))
		})
	}
}

func TestClassifyErrorFromHTTPResponses(t *testing.T) {
	// Exercises the full path: HTTP response -> api.decodeResponseJSON -> sentinel -> classifyError.
	// This is the seam the CLI telemetry actually runs through, so regression here matters more
	// than the unit-level sentinel check above.
	cases := []struct {
		name       string
		statusCode int
		expected   string
	}{
		{"401 unauthorized", http.StatusUnauthorized, "unauthenticated"},
		{"403 forbidden", http.StatusForbidden, "unauthenticated"},
		{"404 not found", http.StatusNotFound, "not_found"},
		{"500 internal server error", http.StatusInternalServerError, "internal_server_error"},
		{"502 bad gateway", http.StatusBadGateway, "internal_server_error"},
		{"503 service unavailable", http.StatusServiceUnavailable, "internal_server_error"},
		{"418 teapot falls through", http.StatusTeapot, "unknown"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.statusCode)
				_, _ = io.WriteString(w, `{"error":"boom"}`)
			}))
			defer server.Close()

			c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				req.URL.Scheme = "http"
				req.URL.Host = strings.TrimPrefix(server.URL, "http://")
				return http.DefaultClient.Do(req)
			})

			_, err := c.GetSandboxInitTemplate()
			require.Error(t, err)
			require.Equal(t, tc.expected, classifyError(err))
		})
	}
}
