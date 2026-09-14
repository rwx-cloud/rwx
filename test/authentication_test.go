package integration_test

import (
	"encoding/pem"
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

func TestAuthenticationWithoutConfiguredToken(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var requests atomic.Int32
			mux := http.NewServeMux()
			mux.HandleFunc("/api/auth/whoami", func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				assert.Empty(t, r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				if status == http.StatusOK {
					_, _ = w.Write([]byte(`{"organization_slug":"proxy-org","token_kind":"service_account","service_account_name":"proxy"}`))
				} else {
					_, _ = w.Write([]byte(`{"error":"Request rejected by server"}`))
				}
			})
			server := httptest.NewTLSServer(mux)
			defer server.Close()

			home := t.TempDir()
			certPath := filepath.Join(home, "server.pem")
			require.NoError(t, os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{
				Type: "CERTIFICATE", Bytes: server.Certificate().Raw,
			}), 0o600))
			t.Setenv("HOME", home)
			t.Setenv("RWX_ACCESS_TOKEN", "")
			t.Setenv("SSL_CERT_FILE", certPath)
			t.Setenv("RWX_HOST", strings.TrimPrefix(server.URL, "https://"))

			result := runMint(t, input{args: []string{"whoami"}})
			require.EqualValues(t, 1, requests.Load())
			if status == http.StatusOK {
				require.Equal(t, 0, result.exitCode, result.stderr)
				require.Contains(t, result.stdout, "proxy-org")
			} else {
				require.Equal(t, 1, result.exitCode)
				require.Contains(t, result.stderr, "Request rejected by server")
				if status == http.StatusUnauthorized {
					require.Contains(t, result.stderr, "rwx login")
					require.Contains(t, result.stderr, "--access-token")
					require.Contains(t, result.stderr, "RWX_ACCESS_TOKEN")
				} else {
					require.NotContains(t, result.stderr, "rwx login")
				}
			}
		})
	}
}
