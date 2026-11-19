package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/accesstoken"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/stretchr/testify/require"
)

func TestAPIClient_DiscoveryDoesNotRequireTokenOrganization(t *testing.T) {
	for _, token := range []string{"", "foreign-org-token"} {
		t.Run(token, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				require.Equal(t, "public-org", r.URL.Query().Get("organization_slug"))
				require.Equal(t, "owner/repo", r.URL.Query().Get("repository_name"))
				if token == "" {
					require.Empty(t, r.Header.Get("Authorization"))
				} else {
					require.Equal(t, "Bearer "+token, r.Header.Get("Authorization"))
				}
				require.Contains(t, []string{"/mint/api/results/latest", "/mint/api/runs"}, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				_, err := w.Write([]byte(`{}`))
				require.NoError(t, err)
			}))
			defer server.Close()
			client, err := api.NewClient(api.Config{
				Host:        strings.TrimPrefix(server.URL, "http://"),
				AccessToken: token, AccessTokenBackend: accesstoken.NewMemoryBackend(),
			})
			require.NoError(t, err)
			transport := client.RoundTripper
			client.RoundTripper = api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				req.URL.Scheme = "http"
				return transport.RoundTrip(req)
			}).RoundTripper
			_, err = client.RunStatus(api.RunStatusConfig{OrganizationSlug: "public-org", RepositoryName: "owner/repo", BranchName: "main"})
			require.NoError(t, err)
			_, err = client.ListRuns(api.ListRunsConfig{OrganizationSlug: "public-org", RepositoryNames: []string{"owner/repo"}})
			require.NoError(t, err)
			require.Equal(t, 2, requests)
		})
	}
}
