package api_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/stretchr/testify/require"
)

func TestGroupPermissionsAPI(t *testing.T) {
	resource := "vault-id"
	for _, tc := range []struct {
		name, method, payload string
		permissions           []api.Permission
	}{
		{"read", "GET", "", nil},
		{"replace", "PUT", `{"permissions":[{"key":"run:read"},{"key":"vault:manage","resource_id":"vault-id"}]}`, []api.Permission{{Key: "run:read"}, {Key: "vault:manage", ResourceID: &resource}}},
		{"clear nil", "PUT", `{"permissions":[]}`, nil},
		{"clear empty", "PUT", `{"permissions":[]}`, []api.Permission{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, tc.method, req.Method)
				require.Equal(t, "/mint/api/groups/group%2Fid/permissions", req.URL.EscapedPath())
				require.Equal(t, "application/json", req.Header.Get("Accept"))
				if tc.method == "PUT" {
					body, err := io.ReadAll(req.Body)
					require.NoError(t, err)
					require.JSONEq(t, tc.payload, string(body))
					require.Equal(t, "application/json", req.Header.Get("Content-Type"))
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"permissions":[{"key":"vault:manage","resource_id":"vault-id"}]}`))}, nil
			})
			var result *api.GroupPermissionsResult
			var err error
			if tc.method == "GET" {
				result, err = client.GetGroupPermissions("group/id")
			} else {
				result, err = client.ReplaceGroupPermissions("group/id", tc.permissions)
			}
			require.NoError(t, err)
			require.Equal(t, []api.Permission{{Key: "vault:manage", ResourceID: &resource}}, result.Permissions)
		})
	}
}

func TestGroupPermissionsAPIErrors(t *testing.T) {
	for _, status := range []int{403, 404, 422} {
		client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"errors":["permission cannot be removed"]}`))}, nil
		})
		for _, replace := range []bool{false, true} {
			var err error
			if replace {
				_, err = client.ReplaceGroupPermissions("id", nil)
			} else {
				_, err = client.GetGroupPermissions("id")
			}
			require.ErrorContains(t, err, "permission cannot be removed")
			if status == 403 {
				require.ErrorIs(t, err, errors.ErrUnauthenticated)
			}
		}
	}
}
