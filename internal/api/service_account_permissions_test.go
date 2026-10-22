package api_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/stretchr/testify/require"
)

func TestServiceAccountPermissionsRequests(t *testing.T) {
	resource := "vault-one"
	for _, method := range []string{"GET", "PUT", "clear"} {
		t.Run(method, func(t *testing.T) {
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, "/mint/api/service_accounts/account%2Fid/permissions", req.URL.String())
				require.Equal(t, "application/json", req.Header.Get("Accept"))
				if method == "GET" {
					require.Equal(t, "GET", req.Method)
				} else {
					require.Equal(t, "PUT", req.Method)
					require.Equal(t, "application/json", req.Header.Get("Content-Type"))
					body, err := io.ReadAll(req.Body)
					require.NoError(t, err)
					expected := `{"permissions":[{"key":"run:read"},{"key":"vault:read","resource_id":"vault-one"}]}`
					if method == "clear" {
						expected = `{"permissions":[]}`
					}
					require.JSONEq(t, expected, string(body))
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"permissions":[{"key":"vault:read","resource_id":"vault-one"}]}`))}, nil
			})
			var result *api.ServiceAccountPermissionsResult
			var err error
			if method == "GET" {
				result, err = client.GetServiceAccountPermissions("account/id")
			} else {
				cfg := api.ReplaceServiceAccountPermissionsConfig{ServiceAccountID: "account/id"}
				if method == "PUT" {
					cfg.Permissions = []api.Permission{{Key: "run:read"}, {Key: "vault:read", ResourceID: &resource}}
				}
				result, err = client.ReplaceServiceAccountPermissions(cfg)
			}
			require.NoError(t, err)
			require.Equal(t, []api.Permission{{Key: "vault:read", ResourceID: &resource}}, result.Permissions)
		})
	}
}
