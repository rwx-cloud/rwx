package cli_test

import (
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

func testVaultRepositoryAccess() api.VaultRepositoryPermission {
	return api.VaultRepositoryPermission{
		ID:             "permission-1",
		Vault:          api.VaultIdentity{ID: "vault-1", Name: "deploys"},
		RepositorySlug: "rwx-cloud/cloud",
		BranchPattern:  "main",
	}
}

func configureVaultAccessResolution(s *testSetup) {
	s.mockAPI.MockListVaults = func() (*api.ListVaultsResult, error) {
		return &api.ListVaultsResult{Vaults: []api.Vault{{ID: "vault-1", Name: "deploys"}}}, nil
	}
}

func TestService_ListVaultAccess(t *testing.T) {
	t.Run("prints repository access as a discriminated JSON grant", func(t *testing.T) {
		s := setupTest(t)
		configureVaultAccessResolution(s)
		s.mockAPI.MockListVaultRepositoryPermissions = func(cfg api.ListVaultRepositoryPermissionsConfig) (*api.ListVaultRepositoryPermissionsResult, error) {
			require.Equal(t, "vault-1", cfg.VaultID)
			return &api.ListVaultRepositoryPermissionsResult{RepositoryPermissions: []api.VaultRepositoryPermission{testVaultRepositoryAccess()}}, nil
		}

		result, err := s.service.ListVaultAccess(cli.ListVaultAccessConfig{Vault: "deploys", Json: true})

		require.NoError(t, err)
		require.Len(t, result.AccessGrants, 1)
		require.JSONEq(t, `{"AccessGrants":[{"ID":"permission-1","Vault":{"ID":"vault-1","Name":"deploys"},"AccessType":"repository","RepositorySlug":"rwx-cloud/cloud","RepositoryBranchPattern":"main"}]}`, s.mockStdout.String())
	})

	t.Run("prints a table shaped for multiple access types", func(t *testing.T) {
		s := setupTest(t)
		configureVaultAccessResolution(s)
		s.mockAPI.MockListVaultRepositoryPermissions = func(cfg api.ListVaultRepositoryPermissionsConfig) (*api.ListVaultRepositoryPermissionsResult, error) {
			return &api.ListVaultRepositoryPermissionsResult{RepositoryPermissions: []api.VaultRepositoryPermission{testVaultRepositoryAccess()}}, nil
		}

		_, err := s.service.ListVaultAccess(cli.ListVaultAccessConfig{Vault: "deploys"})

		require.NoError(t, err)
		require.Contains(t, s.mockStdout.String(), "TYPE        IDENTITY         BRANCH PATTERN")
		require.Contains(t, s.mockStdout.String(), "repository  rwx-cloud/cloud  main")
	})

	t.Run("rejects an ambiguous vault name", func(t *testing.T) {
		s := setupTest(t)
		s.mockAPI.MockListVaults = func() (*api.ListVaultsResult, error) {
			return &api.ListVaultsResult{Vaults: []api.Vault{
				{ID: "vault-1", Name: "deploys"},
				{ID: "vault-2", Name: "deploys"},
			}}, nil
		}

		_, err := s.service.ListVaultAccess(cli.ListVaultAccessConfig{Vault: "deploys"})

		require.ErrorContains(t, err, `more than one vault matches "deploys"; use its ID`)
	})
}

func TestService_AllowVaultAccess(t *testing.T) {
	configure := func(s *testSetup) {
		configureVaultAccessResolution(s)
		s.mockAPI.MockCreateVaultRepositoryPermission = func(cfg api.CreateVaultRepositoryPermissionConfig) (*api.CreateVaultRepositoryPermissionResult, error) {
			require.Equal(t, "vault-1", cfg.VaultID)
			require.Equal(t, "rwx-cloud/cloud", cfg.RepositorySlug)
			require.Equal(t, "release/**", cfg.BranchPattern)
			permission := testVaultRepositoryAccess()
			permission.BranchPattern = cfg.BranchPattern
			return &permission, nil
		}
	}

	t.Run("allows repository access", func(t *testing.T) {
		s := setupTest(t)
		configure(s)

		result, err := s.service.AllowVaultAccess(cli.AllowVaultAccessConfig{
			Vault: "deploys", RepositorySlug: "rwx-cloud/cloud", RepositoryBranchPattern: "release/**",
		})

		require.NoError(t, err)
		require.Equal(t, "repository", result.AccessType)
		require.Contains(t, s.mockStdout.String(), `Allowed runs from repository "rwx-cloud/cloud" matching "release/**" to access vault "deploys".`)
		require.Empty(t, s.mockStderr.String())
	})

	for _, tc := range []struct {
		name      string
		allowFor  string
		expiresAt string
	}{
		{name: "relative expiration", allowFor: "8h"},
		{name: "absolute expiration", expiresAt: "2026-10-31T17:00:00Z"},
	} {
		t.Run("warns and ignores "+tc.name, func(t *testing.T) {
			s := setupTest(t)
			configure(s)

			_, err := s.service.AllowVaultAccess(cli.AllowVaultAccessConfig{
				Vault: "deploys", RepositorySlug: "rwx-cloud/cloud", RepositoryBranchPattern: "release/**",
				AllowFor: tc.allowFor, ExpiresAt: tc.expiresAt, Json: true,
			})

			require.NoError(t, err)
			require.Contains(t, s.mockStderr.String(), "repository access does not support expiration")
			require.JSONEq(t, `{"ID":"permission-1","Vault":{"ID":"vault-1","Name":"deploys"},"AccessType":"repository","RepositorySlug":"rwx-cloud/cloud","RepositoryBranchPattern":"release/**"}`, s.mockStdout.String())
		})
	}

	t.Run("requires both repository selectors", func(t *testing.T) {
		s := setupTest(t)

		_, err := s.service.AllowVaultAccess(cli.AllowVaultAccessConfig{RepositorySlug: "rwx-cloud/cloud"})

		require.ErrorContains(t, err, "--repository-branch-pattern is required")
	})
}

func TestService_RevokeVaultAccess(t *testing.T) {
	configure := func(s *testSetup) {
		configureVaultAccessResolution(s)
		s.mockAPI.MockListVaultRepositoryPermissions = func(cfg api.ListVaultRepositoryPermissionsConfig) (*api.ListVaultRepositoryPermissionsResult, error) {
			other := testVaultRepositoryAccess()
			other.ID = "permission-2"
			other.BranchPattern = "release/**"
			return &api.ListVaultRepositoryPermissionsResult{RepositoryPermissions: []api.VaultRepositoryPermission{
				testVaultRepositoryAccess(),
				other,
			}}, nil
		}
	}

	t.Run("requires confirmation in non-interactive environments", func(t *testing.T) {
		s := setupTest(t)
		configure(s)

		_, err := s.service.RevokeVaultAccess(cli.RevokeVaultAccessConfig{
			Vault: "deploys", RepositorySlug: "rwx-cloud/cloud", RepositoryBranchPattern: "main",
		})

		require.ErrorContains(t, err, "use --yes to confirm")
	})

	t.Run("resolves the exact pair and revokes only that permission", func(t *testing.T) {
		s := setupTest(t)
		configure(s)
		s.mockAPI.MockDeleteVaultRepositoryPermission = func(cfg api.DeleteVaultRepositoryPermissionConfig) (*api.DeleteVaultRepositoryPermissionResult, error) {
			require.Equal(t, "vault-1", cfg.VaultID)
			require.Equal(t, "permission-1", cfg.PermissionID)
			return &api.DeleteVaultRepositoryPermissionResult{}, nil
		}

		result, err := s.service.RevokeVaultAccess(cli.RevokeVaultAccessConfig{
			Vault: "deploys", RepositorySlug: "rwx-cloud/cloud", RepositoryBranchPattern: "main", Yes: true, Json: true,
		})

		require.NoError(t, err)
		require.Equal(t, "permission-1", result.ID)
		require.JSONEq(t, `{"ID":"permission-1","Vault":{"ID":"vault-1","Name":"deploys"},"AccessType":"repository","RepositorySlug":"rwx-cloud/cloud","RepositoryBranchPattern":"main"}`, s.mockStdout.String())
	})

	t.Run("reports a missing repository grant", func(t *testing.T) {
		s := setupTest(t)
		configure(s)

		_, err := s.service.RevokeVaultAccess(cli.RevokeVaultAccessConfig{
			Vault: "deploys", RepositorySlug: "rwx-cloud/cloud", RepositoryBranchPattern: "missing", Yes: true,
		})

		require.ErrorContains(t, err, `repository access for "rwx-cloud/cloud" with branch pattern "missing" was not found`)
	})
}
