package cli_test

import (
	"errors"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestServiceListPermissions(t *testing.T) {
	for _, target := range []cli.ListPermissionsConfig{{}, {GroupID: "group-one"}, {ServiceAccountID: "account-two"}} {
		for _, asJSON := range []bool{false, true} {
			s := setupTest(t)
			resource := "vault"
			s.mockAPI.MockGetPermissionCatalog = func(cfg api.GetPermissionCatalogConfig) (*api.PermissionCatalog, error) {
				require.Equal(t, target.GroupID, cfg.GroupID)
				require.Equal(t, target.ServiceAccountID, cfg.ServiceAccountID)
				return &api.PermissionCatalog{
					Permissions: []api.CatalogPermission{
						{Key: "vault:manage", SupportsResourceScope: true, ScopeResource: &resource, Grantable: api.PermissionGrantability{ResourceIDs: []string{"vault-one"}}},
						{Key: "run:read", Grantable: api.PermissionGrantability{Unscoped: true}},
						{Key: "future:permission"},
					},
					UnremovablePermissions: []string{"vault:manage(id: vault-two)"},
				}, nil
			}
			target.Json = asJSON
			result, err := s.service.ListPermissions(target)
			require.NoError(t, err)
			require.Len(t, result.Permissions, 3)
			if asJSON {
				require.JSONEq(t, `{
					"Permissions":[
						{"Key":"vault:manage","SupportsResourceScope":true,"ScopeResource":"vault","Grantable":{"Unscoped":false,"ResourceIDs":["vault-one"]}},
						{"Key":"run:read","SupportsResourceScope":false,"ScopeResource":null,"Grantable":{"Unscoped":true,"ResourceIDs":[]}},
						{"Key":"future:permission","SupportsResourceScope":false,"ScopeResource":null,"Grantable":{"Unscoped":false,"ResourceIDs":[]}}
					],"UnremovablePermissions":["vault:manage(id: vault-two)"]
				}`, s.mockStdout.String())
			} else {
				output := s.mockStdout.String()
				require.Contains(t, output, "CALLER CAN GRANT UNSCOPED")
				require.Regexp(t, `vault:manage\s+true\s+vault\s+false\s+vault-one`, output)
				require.Regexp(t, `run:read\s+false\s+-\s+true\s+-`, output)
				require.Contains(t, output, "future:permission")
				require.Contains(t, output, "Target permissions the caller cannot remove:\nvault:manage(id: vault-two)")
			}
		}
	}
}

func TestServiceListPermissionsEmpty(t *testing.T) {
	for _, asJSON := range []bool{false, true} {
		s := setupTest(t)
		s.mockAPI.MockGetPermissionCatalog = func(cfg api.GetPermissionCatalogConfig) (*api.PermissionCatalog, error) {
			return &api.PermissionCatalog{}, nil
		}
		_, err := s.service.ListPermissions(cli.ListPermissionsConfig{Json: asJSON})
		require.NoError(t, err)
		if asJSON {
			require.JSONEq(t, `{"Permissions":[],"UnremovablePermissions":[]}`, s.mockStdout.String())
		} else {
			require.Equal(t, "No permissions found.\n", s.mockStdout.String())
		}
	}
}

func TestServiceListPermissionsErrors(t *testing.T) {
	s := setupTest(t)
	_, err := s.service.ListPermissions(cli.ListPermissionsConfig{GroupID: "one", ServiceAccountID: "two"})
	require.ErrorContains(t, err, "cannot be used together")
	failure := errors.New("permission denied")
	s.mockAPI.MockGetPermissionCatalog = func(cfg api.GetPermissionCatalogConfig) (*api.PermissionCatalog, error) {
		return nil, failure
	}
	result, err := s.service.ListPermissions(cli.ListPermissionsConfig{})
	require.Nil(t, result)
	require.ErrorIs(t, err, failure)
	require.ErrorContains(t, err, "unable to list permissions")
	require.Empty(t, s.mockStdout.String())
}
