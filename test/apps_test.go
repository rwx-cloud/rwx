package integration_test

import (
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppsShare(t *testing.T) {
	var requests atomic.Int32
	var mu sync.Mutex
	var endpoint, description string
	var duration time.Duration
	var started time.Time
	status := http.StatusCreated
	const shareURL = "https://pr-123--example.rwx.app/rwx-share?code=test-code"
	const expiresAt = "2030-01-02T03:04:05Z"
	mux := http.NewServeMux()
	mux.HandleFunc("/mint/api/preview_app_shares", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requests.Add(1)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/mint/api/preview_app_shares", r.URL.RequestURI())
		assert.Equal(t, "Bearer fake-for-test", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		var body map[string]string
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
			return
		}
		assert.Equal(t, endpoint, body["endpoint"])
		assert.Equal(t, description, body["description"])
		expiry, err := time.Parse(time.RFC3339Nano, body["expires_at"])
		assert.NoError(t, err)
		assert.False(t, expiry.Before(started.Add(duration)))
		assert.False(t, expiry.After(time.Now().Add(duration)))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status != http.StatusCreated {
			_, _ = io.WriteString(w, `{"error_messages":[{"message":"This preview is no longer available for sharing."}]}`)
			return
		}
		// Use the server's metadata, rather than echoing the request.
		var savedDescription any
		if description != "" {
			savedDescription = "Saved description"
		}
		assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"share_url": shareURL, "expires_at": expiresAt, "description": savedDescription,
		}))
	})
	server := httptest.NewTLSServer(mux)
	defer server.Close()
	home := t.TempDir()
	certPath := filepath.Join(home, "server.pem")
	require.NoError(t, os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE", Bytes: server.Certificate().Raw,
	}), 0o600))
	t.Setenv("HOME", home)
	t.Setenv("RWX_ACCESS_TOKEN", "fake-for-test")
	t.Setenv("SSL_CERT_FILE", certPath)
	t.Setenv("RWX_HOST", strings.TrimPrefix(server.URL, "https://"))

	for _, tc := range []struct {
		name, endpoint, description string
		flags                       []string
		duration                    time.Duration
		json                        bool
	}{
		{"defaults", "pr-123", "", nil, 168 * time.Hour, false},
		{"URL and compound duration", "https://pr-123--example.rwx.app/path?q=value", `Review "new design"`, []string{"--expires-in", "1h30m", "--output", "json"}, 90 * time.Minute, true},
		{"days", "pr-123", "", []string{"--expires-in", "7d", "--json"}, 168 * time.Hour, true},
		{"longer than one week", "pr-123", "", []string{"--expires-in", "30d", "--json"}, 720 * time.Hour, true},
		{"fractional days", "pr-123", "", []string{"--expires-in", "1.5d", "--json"}, 36 * time.Hour, true},
		{"days and hours", "pr-123", "", []string{"--expires-in", "1d2h", "--json"}, 26 * time.Hour, true},
		{"hours and days", "pr-123", "", []string{"--expires-in", "1h2d", "--json"}, 49 * time.Hour, true},
		{"fractional duration", "pr-123", "", []string{"--expires-in", "1.5h", "--format", "json"}, 90 * time.Minute, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mu.Lock()
			endpoint, description, duration = tc.endpoint, tc.description, tc.duration
			args := append([]string{"apps", "share", endpoint}, tc.flags...)
			if description != "" {
				args = append(args, "--description", description)
			}
			started = time.Now()
			mu.Unlock()
			result := runMint(t, input{args: args})
			require.Equal(t, 0, result.exitCode, result.stderr)
			if tc.json {
				wantDescription := ""
				if tc.description != "" {
					wantDescription = "Saved description"
				}
				var output map[string]any
				require.NoError(t, json.Unmarshal([]byte(result.stdout), &output))
				require.Equal(t, map[string]any{
					"URL": shareURL, "ExpiresAt": expiresAt, "Description": wantDescription,
				}, output)
			} else {
				require.Equal(t, shareURL, result.stdout)
			}
		})
	}
	require.EqualValues(t, 8, requests.Load())
	for _, value := range []string{"", "0", "-1h", "0d", "-1d", "1d-2h", "1..5d", "1day", "1e3d", "tomorrow", "999999999999h", "106752d"} {
		result := runMint(t, input{args: []string{"apps", "share", "pr-123", "--expires-in", value}})
		require.Equal(t, 1, result.exitCode)
		require.Contains(t, result.stderr, "--expires-in must be")
		require.Empty(t, result.stdout)
	}
	for _, names := range [][]string{nil, {"pr-123", "pr-456"}} {
		result := runMint(t, input{args: append([]string{"apps", "share"}, names...)})
		require.Equal(t, 1, result.exitCode)
		require.Contains(t, result.stderr, "accepts 1 arg(s)")
	}
	require.EqualValues(t, 8, requests.Load(), "invalid arguments must not create shares")

	mu.Lock()
	status = http.StatusUnprocessableEntity
	endpoint, description, duration = "pr-123", "", 168*time.Hour
	started = time.Now()
	mu.Unlock()
	result := runMint(t, input{args: []string{"apps", "share", endpoint}})
	require.Equal(t, 1, result.exitCode)
	require.Contains(t, result.stderr, `unable to share app endpoint "pr-123"`)
	require.Contains(t, result.stderr, "This preview is no longer available for sharing.")
	require.Empty(t, result.stdout)
	require.EqualValues(t, 9, requests.Load())
}

func TestApps(t *testing.T) {
	for _, action := range []string{"disable", "enable"} {
		t.Run(action, func(t *testing.T) {
			var requests atomic.Int32
			mux := http.NewServeMux()
			mux.HandleFunc("/mint/api/app_endpoints/"+action, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/mint/api/app_endpoints/"+action, r.URL.RequestURI())
				assert.Equal(t, "Bearer fake-for-test", r.Header.Get("Authorization"))
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.JSONEq(t, `{"endpoint":"pr-123"}`, string(body))
				w.WriteHeader(http.StatusNoContent)
			})
			server := httptest.NewTLSServer(mux)
			defer server.Close()

			home := t.TempDir()
			certPath := filepath.Join(home, "server.pem")
			require.NoError(t, os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{
				Type: "CERTIFICATE", Bytes: server.Certificate().Raw,
			}), 0o600))
			t.Setenv("HOME", home)
			t.Setenv("RWX_ACCESS_TOKEN", "fake-for-test")
			t.Setenv("SSL_CERT_FILE", certPath)
			t.Setenv("RWX_HOST", strings.TrimPrefix(server.URL, "https://"))

			for _, flags := range [][]string{{"--format", "json"}, {"--output", "json"}, {"--json"}} {
				args := append([]string{"apps", action, "pr-123"}, flags...)
				result := runMint(t, input{args: args})
				require.Equal(t, 0, result.exitCode, result.stderr)
				if action == "disable" {
					require.JSONEq(t, `{"Endpoint":"pr-123","Disabled":true}`, result.stdout)
				} else {
					require.JSONEq(t, `{"Endpoint":"pr-123","Disabled":false}`, result.stdout)
				}
			}
			require.EqualValues(t, 3, requests.Load())

			for _, names := range [][]string{nil, {"pr-123", "pr-456"}} {
				result := runMint(t, input{args: append([]string{"apps", action}, names...)})
				require.Equal(t, 1, result.exitCode)
				require.Contains(t, result.stderr, "accepts 1 arg(s)")
			}
			require.EqualValues(t, 3, requests.Load(), "invalid arguments must not send a request")
		})
	}
}
