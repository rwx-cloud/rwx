package integration_test

import (
	"encoding/json"
	"encoding/pem"
	"fmt"
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

func TestVaultToolCaches(t *testing.T) {
	var mu sync.Mutex
	evictions := 0
	listStatus, evictStatus := http.StatusOK, http.StatusOK
	empty := false
	caches := []map[string]any{
		{"id": "cache-1", "name": "build", "installation_id": "install-1", "os": "linux", "arch": "x86_64", "populated": true, "last_read_at": nil, "last_written_at": "2026-11-23T12:00:00Z", "last_evicted_at": nil, "expiry": "2026-11-25T12:00:00Z"},
		{"id": "cache-2", "name": "build", "installation_id": "install-2", "os": "linux", "arch": "arm64", "populated": true, "last_read_at": nil, "last_written_at": nil, "last_evicted_at": nil, "expiry": nil},
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		assert.Equal(t, "Bearer fake-for-test", r.Header.Get("Authorization"))
		switch {
		case r.URL.Path == "/api/telemetry":
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/api/skill/latest":
			w.WriteHeader(http.StatusNotFound)
		case r.URL.Path == "/mint/api/vaults":
			assert.Equal(t, http.MethodGet, r.Method)
			_, _ = fmt.Fprint(w, `{"vaults":[{"id":"vault-1","name":"test-vault"}]}`)
		case r.URL.Path == "/mint/api/vaults/vault-1/tool_caches":
			assert.Equal(t, http.MethodGet, r.Method)
			if listStatus != http.StatusOK {
				w.WriteHeader(listStatus)
				_, _ = fmt.Fprint(w, `{"error_messages":[{"message":"listing denied"}]}`)
				return
			}
			if empty {
				_, _ = fmt.Fprint(w, `{"tool_caches":[],"pagination":{"next_cursor":null,"limit":50}}`)
				return
			}
			var page []map[string]any
			var cursor any
			if r.URL.Query().Get("cursor") == "" {
				page, cursor = caches[:1], "next+/="
			} else {
				assert.Equal(t, "next+/=", r.URL.Query().Get("cursor"))
				page = caches[1:]
			}
			assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"tool_caches": page, "pagination": map[string]any{"next_cursor": cursor, "limit": 50},
			}))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/evict"):
			evictions++
			if evictStatus != http.StatusOK {
				w.WriteHeader(evictStatus)
				_, _ = fmt.Fprint(w, `{"error_messages":[{"message":"eviction denied"}],"required_permission":"vault:evict_tool_cache"}`)
				return
			}
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/mint/api/vaults/vault-1/tool_caches/"), "/evict")
			for _, cache := range caches {
				if cache["id"] == id {
					cache["populated"] = false
					cache["last_evicted_at"] = "2026-11-23T13:00:00Z"
					cache["expiry"] = "2026-11-23T13:00:00Z"
					assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"tool_cache": cache}))
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `{"error_messages":[{"message":"tool cache ID not found"}]}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	home := t.TempDir()
	certPath := filepath.Join(home, "server.pem")
	require.NoError(t, os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600))
	t.Setenv("HOME", home)
	t.Setenv("RWX_ACCESS_TOKEN", "fake-for-test")
	t.Setenv("SSL_CERT_FILE", certPath)
	t.Setenv("RWX_HOST", strings.TrimPrefix(server.URL, "https://"))

	for _, flags := range [][]string{{"--json"}, {"--format", "json"}, {"--output", "json"}} {
		result := runMint(t, input{args: append([]string{"vaults", "tool-caches", "list", "--vault", "test-vault"}, flags...)})
		require.Equal(t, 0, result.exitCode, result.stderr)
		require.JSONEq(t, `{"ToolCaches":[
			{"ID":"cache-1","Name":"build","InstallationID":"install-1","OS":"linux","Arch":"x86_64","Populated":true,"LastReadAt":null,"LastWrittenAt":"2026-11-23T12:00:00Z","LastEvictedAt":null,"Expiry":"2026-11-25T12:00:00Z"},
			{"ID":"cache-2","Name":"build","InstallationID":"install-2","OS":"linux","Arch":"arm64","Populated":true,"LastReadAt":null,"LastWrittenAt":null,"LastEvictedAt":null,"Expiry":null}
		]}`, result.stdout)
	}
	listed := runMint(t, input{args: []string{"vaults", "tool-caches", "list", "--vault", "test-vault"}})
	require.Equal(t, 0, listed.exitCode, listed.stderr)
	for _, column := range []string{"ID", "NAME", "INSTALLATION ID", "OS", "ARCH", "POPULATED", "install-2", "cache-1", "cache-2"} {
		require.Contains(t, listed.stdout, column)
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"evict", "cache-2", "--vault", "test-vault"}, "use --yes"},
		{[]string{"list"}, `required flag(s) "vault" not set`},
		{[]string{"evict", "cache-2"}, `required flag(s) "vault" not set`},
		{[]string{"evict", "--vault", "test-vault"}, "accepts 1 arg(s)"},
		{[]string{"evict", "cache-1", "cache-2", "--vault", "test-vault"}, "accepts 1 arg(s)"},
	} {
		result := runMint(t, input{args: append([]string{"vaults", "tool-caches"}, tc.args...)})
		require.Equal(t, 1, result.exitCode)
		require.Contains(t, result.stderr, tc.want)
		require.Empty(t, result.stdout)
	}
	mu.Lock()
	evictionCount := evictions
	listStatus = http.StatusForbidden
	mu.Unlock()
	require.Zero(t, evictionCount)

	evicted := runMint(t, input{args: []string{"vaults", "tool-caches", "evict", "cache-2", "--vault", "vault-1", "--yes", "--json"}})
	require.Equal(t, 0, evicted.exitCode, evicted.stderr)
	require.JSONEq(t, `{"ID":"cache-2","Name":"build","InstallationID":"install-2","OS":"linux","Arch":"arm64","Populated":false,"LastReadAt":null,"LastWrittenAt":null,"LastEvictedAt":"2026-11-23T13:00:00Z","Expiry":"2026-11-23T13:00:00Z"}`, evicted.stdout)
	repeated := runMint(t, input{args: []string{"vaults", "tool-caches", "evict", "cache-2", "--vault", "test-vault", "--yes"}})
	require.Equal(t, 0, repeated.exitCode, repeated.stderr)
	require.Contains(t, repeated.stdout, `Evicted tool cache "build" (cache-2)`)
	for _, id := range []string{"build", "absent"} {
		missing := runMint(t, input{args: []string{"vaults", "tool-caches", "evict", id, "--vault", "test-vault", "--yes", "--json"}})
		require.Equal(t, 1, missing.exitCode)
		require.Contains(t, missing.stderr, "tool cache ID not found")
		require.Empty(t, missing.stdout)
	}
	mu.Lock()
	listStatus = http.StatusOK
	mu.Unlock()
	listed = runMint(t, input{args: []string{"vaults", "tool-caches", "list", "--vault", "test-vault", "--json"}})
	require.Equal(t, 0, listed.exitCode, listed.stderr)
	var output struct {
		ToolCaches []struct {
			ID        string
			Populated bool
		}
	}
	require.NoError(t, json.Unmarshal([]byte(listed.stdout), &output))
	require.True(t, output.ToolCaches[0].Populated)
	require.False(t, output.ToolCaches[1].Populated)

	mu.Lock()
	evictionCount = evictions
	evictStatus = http.StatusForbidden
	mu.Unlock()
	require.Equal(t, 4, evictionCount)
	denied := runMint(t, input{args: []string{"vaults", "tool-caches", "evict", "cache-1", "--vault", "test-vault", "--yes", "--json"}})
	require.Equal(t, 1, denied.exitCode)
	require.Contains(t, denied.stderr, "eviction denied")
	require.Empty(t, denied.stdout)
	mu.Lock()
	listStatus = http.StatusForbidden
	mu.Unlock()
	denied = runMint(t, input{args: []string{"vaults", "tool-caches", "list", "--vault", "test-vault", "--json"}})
	require.Equal(t, 1, denied.exitCode)
	require.Contains(t, denied.stderr, "listing denied")
	require.Empty(t, denied.stdout)
	mu.Lock()
	listStatus = http.StatusOK
	empty = true
	mu.Unlock()
	listed = runMint(t, input{args: []string{"vaults", "tool-caches", "list", "--vault", "test-vault", "--json"}})
	require.Equal(t, 0, listed.exitCode, listed.stderr)
	require.JSONEq(t, `{"ToolCaches":[]}`, listed.stdout)
}
