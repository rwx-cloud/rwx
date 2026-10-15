package integration_test

import (
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBillingUsageMockAPI(t *testing.T) {
	fixture, err := os.ReadFile("../internal/api/testdata/billing_usage_api.json")
	require.NoError(t, err)
	requests := make(chan *http.Request, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/billing/usage", func(w http.ResponseWriter, r *http.Request) {
		requests <- r
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("month") == "9999-12" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write(fixture)
	})
	server := httptest.NewTLSServer(mux)
	defer server.Close()
	certFile := filepath.Join(t.TempDir(), "server.pem")
	require.NoError(t, os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600))
	t.Setenv("SSL_CERT_FILE", certFile)
	t.Setenv("RWX_HOST", strings.TrimPrefix(server.URL, "https://"))
	for _, tc := range []struct {
		name   string
		flags  []string
		month  string
		json   bool
		future bool
	}{
		{name: "default text"},
		{name: "explicit json", flags: []string{"--month", "2026-09", "--json"}, month: "2026-09", json: true},
		{name: "format json", flags: []string{"--format", "json"}, json: true},
		{name: "legacy output json", flags: []string{"--output", "json"}, json: true},
		{name: "server validation", flags: []string{"--month", "9999-12"}, month: "9999-12", future: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"billing", "usage", "--access-token", "fake-for-test"}, tc.flags...)
			result := runMint(t, input{args: args})
			require.Len(t, requests, 1, result.stderr)
			req := <-requests
			require.Equal(t, http.MethodGet, req.Method)
			require.Equal(t, "Bearer fake-for-test", req.Header.Get("Authorization"))
			require.Equal(t, tc.month, req.URL.Query().Get("month"))
			if tc.month == "" {
				require.Empty(t, req.URL.RawQuery)
			}
			if tc.future {
				require.Equal(t, 1, result.exitCode)
				require.Empty(t, result.stdout)
				require.Contains(t, result.stderr, "Unable to call RWX API - 400 Bad Request")
				return
			}
			require.Zero(t, result.exitCode, result.stderr)
			if tc.json {
				var output map[string]any
				require.NoError(t, json.Unmarshal([]byte(result.stdout), &output))
				require.Equal(t, "82.469135780246913578", output["UsageCostInDollars"])
				require.Equal(t, "50.123456789", output["AvailableCreditInDollars"])
				require.Equal(t, "2026-09-01", output["Period"].(map[string]any)["FinalizedThrough"])
				require.Len(t, output["Days"].([]any), 4)
				require.Nil(t, output["Days"].([]any)[1].(map[string]any)["CostInDollars"])
			} else {
				require.Contains(t, result.stdout, "Usage cost: 82.469135780246913578")
				require.Contains(t, result.stdout, "123456.7890123456789")
				require.Contains(t, result.stdout, "Pending")
				require.Contains(t, result.stdout, "Available")
			}
		})
	}
}

func TestBillingUsageValidation(t *testing.T) {
	for _, month := range []string{"2026-9", "2026-13", "2026-09-01", ""} {
		t.Run(month, func(t *testing.T) {
			result := runMint(t, input{args: []string{"billing", "usage", "--month", month, "--access-token", "fake-for-test"}})
			require.Equal(t, 1, result.exitCode)
			require.Empty(t, result.stdout)
			require.Contains(t, result.stderr, "month must be in YYYY-MM format")
		})
	}
}
