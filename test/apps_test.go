package integration_test

import (
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
