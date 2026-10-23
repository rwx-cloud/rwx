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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResultsRepresentations(t *testing.T) {
	body, err := os.ReadFile("../internal/api/testdata/compact_results.json")
	require.NoError(t, err)
	var fixture map[string][]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &fixture))
	for _, tc := range []struct {
		name                 string
		flags                []string
		verbose, unsupported bool
	}{
		{"compact default", []string{"--json"}, false, false},
		{"format compact default", []string{"--format", "json"}, false, false},
		{"legacy escape hatch", []string{"--json", "--verbose"}, true, false},
		{"before Cloud deployment", []string{"--json"}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			pages := map[string]int{}
			mux := http.NewServeMux()
			mux.HandleFunc("/mint/api/results/status", func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "run", r.URL.Query().Get("id"))
				_, _ = io.WriteString(w, `{"run_status":{"result":"succeeded"},"polling":{"completed":true}}`)
			})
			mux.HandleFunc("/mint/api/results/prompt", func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "run", r.URL.Query().Get("id"))
				_, _ = io.WriteString(w, "unchanged prompt")
			})
			mux.HandleFunc("/mint/api/results/", func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				id := strings.TrimPrefix(r.URL.Path, "/mint/api/results/")
				if tc.verbose || tc.unsupported {
					if tc.verbose {
						assert.Empty(t, r.URL.RawQuery)
					} else {
						assert.Equal(t, "compact", r.URL.Query().Get("view"))
					}
					pages[id]++
					_, _ = io.WriteString(w, `{"id":"run","env":{"GIT_FLAG":"on"}}`)
					return
				}
				assert.Equal(t, "compact", r.URL.Query().Get("view"))
				index := pages[id]
				if !assert.Less(t, index, len(fixture[id])) {
					w.WriteHeader(500)
					return
				}
				pages[id]++
				_, _ = w.Write(fixture[id][index])
			})
			server := httptest.NewTLSServer(mux)
			defer server.Close()
			home := t.TempDir()
			certPath := filepath.Join(home, "server.pem")
			require.NoError(t, os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600))
			t.Setenv("HOME", home)
			t.Setenv("RWX_ACCESS_TOKEN", "fake-for-test")
			t.Setenv("SSL_CERT_FILE", certPath)
			t.Setenv("RWX_HOST", strings.TrimPrefix(server.URL, "https://"))
			result := runMint(t, input{args: append([]string{"results", "run"}, tc.flags...)})
			if tc.unsupported {
				require.Equal(t, 1, result.exitCode)
				require.Empty(t, result.stdout)
				require.Contains(t, result.stderr, "use --verbose")
				return
			}
			require.Equal(t, 0, result.exitCode, result.stderr)
			var output map[string]any
			require.NoError(t, json.Unmarshal([]byte(result.stdout), &output))
			require.Equal(t, "run", output["RunID"])
			require.Equal(t, "unchanged prompt", output["ResultPrompt"])
			require.Equal(t, "succeeded", output["ResultStatus"])
			mu.Lock()
			defer mu.Unlock()
			if tc.verbose {
				require.Equal(t, map[string]any{"GIT_FLAG": "on"}, output["Env"])
				require.Equal(t, map[string]int{"run": 1}, pages)
			} else {
				require.NotContains(t, output, "Env")
				require.Equal(t, map[string]int{"run": 3, "group-old": 2, "leaf-old": 1, "a-release": 1}, pages)
				tasks := output["Tasks"].([]any)
				require.Equal(t, "group-old", tasks[0].(map[string]any)["ID"])
				require.Equal(t, "a-release", tasks[1].(map[string]any)["ID"])
			}
		})
	}
}
