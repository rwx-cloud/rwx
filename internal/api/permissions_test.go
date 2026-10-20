package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	internalErrors "github.com/rwx-cloud/rwx/internal/errors"
	"github.com/stretchr/testify/require"
)

func TestGetPermissionCatalog(t *testing.T) {
	for _, cfg := range []api.GetPermissionCatalogConfig{
		{}, {GroupID: "group &/one"}, {ServiceAccountID: "account &/two"},
	} {
		t.Run(cfg.GroupID+cfg.ServiceAccountID, func(t *testing.T) {
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, http.MethodGet, req.Method)
				require.Equal(t, "/mint/api/permission_catalog", req.URL.Path)
				query := url.Values{}
				if cfg.GroupID != "" {
					query.Set("group_id", cfg.GroupID)
				}
				if cfg.ServiceAccountID != "" {
					query.Set("service_account_id", cfg.ServiceAccountID)
				}
				require.Equal(t, query, req.URL.Query())
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{
					"permissions":[
						{"key":"vault:manage","supports_resource_scope":true,"scope_resource":"vault","grantable":{"unscoped":false,"resource_ids":["vault-one"]}},
						{"key":"run:read","supports_resource_scope":false,"scope_resource":null,"grantable":{"unscoped":true,"resource_ids":[]}}
					],"unremovable_permissions":["vault:manage(id: vault-two)"]
				}`))}, nil
			})
			result, err := client.GetPermissionCatalog(cfg)
			require.NoError(t, err)
			require.Len(t, result.Permissions, 2)
			require.Equal(t, "vault:manage", result.Permissions[0].Key)
			require.True(t, result.Permissions[0].SupportsResourceScope)
			require.Equal(t, "vault", *result.Permissions[0].ScopeResource)
			require.False(t, result.Permissions[0].Grantable.Unscoped)
			require.Equal(t, []string{"vault-one"}, result.Permissions[0].Grantable.ResourceIDs)
			require.False(t, result.Permissions[1].SupportsResourceScope)
			require.Nil(t, result.Permissions[1].ScopeResource)
			require.True(t, result.Permissions[1].Grantable.Unscoped)
			require.Equal(t, []string{}, result.Permissions[1].Grantable.ResourceIDs)
			require.Equal(t, []string{"vault:manage(id: vault-two)"}, result.UnremovablePermissions)
		})
	}
}

func TestGetPermissionCatalogErrors(t *testing.T) {
	for _, tc := range []struct {
		status   int
		body     string
		want     string
		sentinel error
	}{
		{401, `{"errors":["Unauthorized"]}`, "Unauthorized", internalErrors.ErrUnauthenticated},
		{403, `{"errors":["group:manage required"]}`, "group:manage required", internalErrors.ErrUnauthenticated},
		{404, `{"errors":["Group not found"]}`, "Group not found", api.ErrNotFound},
		{422, `{"errors":["Invalid target"]}`, "Invalid target", nil},
		{500, `{"errors":["Unavailable"]}`, "Unavailable", internalErrors.ErrInternalServerError},
		{200, `{`, "unable to parse API response", nil},
	} {
		t.Run(tc.want, func(t *testing.T) {
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			result, err := client.GetPermissionCatalog(api.GetPermissionCatalogConfig{})
			require.Nil(t, result)
			require.ErrorContains(t, err, tc.want)
			if tc.sentinel != nil {
				require.ErrorIs(t, err, tc.sentinel)
			}
		})
	}
	client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
		t.Fatal("conflicting targets must not send a request")
		return nil, nil
	})
	_, err := client.GetPermissionCatalog(api.GetPermissionCatalogConfig{GroupID: "one", ServiceAccountID: "two"})
	require.ErrorContains(t, err, "not both")
}

func TestPermissionWireFormat(t *testing.T) {
	id := "vault-one"
	encoded, err := json.Marshal([]api.Permission{{Key: "run:read"}, {Key: "vault:manage", ResourceID: &id}})
	require.NoError(t, err)
	require.JSONEq(t, `[{"key":"run:read"},{"key":"vault:manage","resource_id":"vault-one"}]`, string(encoded))
}
