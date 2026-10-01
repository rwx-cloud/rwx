package cli_test

import (
	"testing"
	"time"

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
	s.mockAPI.MockListVaultAccessGrants = func(cfg api.ListVaultAccessGrantsConfig) (*api.ListVaultAccessGrantsResult, error) {
		return &api.ListVaultAccessGrantsResult{}, nil
	}
}

func testVaultPrincipalAccess() []api.VaultAccessGrant {
	expiresAt := "2026-10-31T17:00:00.123456Z"
	return []api.VaultAccessGrant{
		{
			ID: "identity-1", Vault: api.VaultIdentity{ID: "vault-1", Name: "deploys"}, ExpiresAt: &expiresAt,
			Principal: api.VaultAccessPrincipal{Type: "user", ID: "user-1", Email: "person@example.com"},
		},
		{
			ID: "identity-2", Vault: api.VaultIdentity{ID: "vault-1", Name: "deploys"},
			Principal: api.VaultAccessPrincipal{Type: "service_account", ID: "account-1", Name: "deploy-bot"},
		},
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

	t.Run("combines user, service account, and repository access", func(t *testing.T) {
		s := setupTest(t)
		configureVaultAccessResolution(s)
		s.mockAPI.MockListVaultAccessGrants = func(cfg api.ListVaultAccessGrantsConfig) (*api.ListVaultAccessGrantsResult, error) {
			require.Equal(t, "vault-1", cfg.VaultID)
			return &api.ListVaultAccessGrantsResult{AccessGrants: testVaultPrincipalAccess()}, nil
		}
		s.mockAPI.MockListVaultRepositoryPermissions = func(cfg api.ListVaultRepositoryPermissionsConfig) (*api.ListVaultRepositoryPermissionsResult, error) {
			return &api.ListVaultRepositoryPermissionsResult{RepositoryPermissions: []api.VaultRepositoryPermission{testVaultRepositoryAccess()}}, nil
		}

		_, err := s.service.ListVaultAccess(cli.ListVaultAccessConfig{Vault: "deploys", Json: true})

		require.NoError(t, err)
		require.JSONEq(t, `{"AccessGrants":[{"ID":"identity-1","Vault":{"ID":"vault-1","Name":"deploys"},"AccessType":"user","UserID":"user-1","Email":"person@example.com","ExpiresAt":"2026-10-31T17:00:00.123456Z"},{"ID":"identity-2","Vault":{"ID":"vault-1","Name":"deploys"},"AccessType":"service_account","ServiceAccountID":"account-1","ServiceAccountName":"deploy-bot","ExpiresAt":null},{"ID":"permission-1","Vault":{"ID":"vault-1","Name":"deploys"},"AccessType":"repository","RepositorySlug":"rwx-cloud/cloud","RepositoryBranchPattern":"main"}]}`, s.mockStdout.String())
	})

	t.Run("prints mixed access in text", func(t *testing.T) {
		s := setupTest(t)
		configureVaultAccessResolution(s)
		s.mockAPI.MockListVaultAccessGrants = func(cfg api.ListVaultAccessGrantsConfig) (*api.ListVaultAccessGrantsResult, error) {
			return &api.ListVaultAccessGrantsResult{AccessGrants: testVaultPrincipalAccess()}, nil
		}
		s.mockAPI.MockListVaultRepositoryPermissions = func(cfg api.ListVaultRepositoryPermissionsConfig) (*api.ListVaultRepositoryPermissionsResult, error) {
			return &api.ListVaultRepositoryPermissionsResult{RepositoryPermissions: []api.VaultRepositoryPermission{testVaultRepositoryAccess()}}, nil
		}

		_, err := s.service.ListVaultAccess(cli.ListVaultAccessConfig{Vault: "deploys"})

		require.NoError(t, err)
		require.Contains(t, s.mockStdout.String(), "user             person@example.com")
		require.Contains(t, s.mockStdout.String(), "2026-10-31T17:00:00.123456Z")
		require.Contains(t, s.mockStdout.String(), "service_account  deploy-bot")
		require.Contains(t, s.mockStdout.String(), "never")
		require.Contains(t, s.mockStdout.String(), "repository       rwx-cloud/cloud")
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

	t.Run("upserts user expiration by email", func(t *testing.T) {
		s := setupTest(t)
		configureVaultAccessResolution(s)
		s.mockAPI.MockCreateVaultAccessGrant = func(cfg api.CreateVaultAccessGrantConfig) (*api.CreateVaultAccessGrantResult, error) {
			require.Equal(t, "vault-1", cfg.VaultID)
			require.Equal(t, "user", cfg.PrincipalType)
			require.Equal(t, "person@example.com", cfg.Email)
			require.Equal(t, "2026-10-31T17:00:00Z", *cfg.ExpiresAt)
			return &api.CreateVaultAccessGrantResult{AccessGrant: testVaultPrincipalAccess()[0]}, nil
		}

		result, err := s.service.AllowVaultAccess(cli.AllowVaultAccessConfig{
			Vault: "deploys", Email: "person@example.com", ExpiresAt: "2026-10-31T17:00:00Z", Json: true,
		})

		require.NoError(t, err)
		require.Equal(t, "user", result.AccessType)
		require.Contains(t, s.mockStdout.String(), `"Email":"person@example.com"`)
	})

	t.Run("upserts service account expiration from a duration", func(t *testing.T) {
		s := setupTest(t)
		configureVaultAccessResolution(s)
		before := time.Now().Add(7*time.Hour + 59*time.Minute)
		s.mockAPI.MockCreateVaultAccessGrant = func(cfg api.CreateVaultAccessGrantConfig) (*api.CreateVaultAccessGrantResult, error) {
			require.Equal(t, "service_account", cfg.PrincipalType)
			require.Equal(t, "deploy-bot", cfg.ServiceAccount)
			expiresAt, err := time.Parse(time.RFC3339Nano, *cfg.ExpiresAt)
			require.NoError(t, err)
			require.True(t, expiresAt.After(before))
			require.True(t, expiresAt.Before(time.Now().Add(8*time.Hour+time.Minute)))
			return &api.CreateVaultAccessGrantResult{AccessGrant: testVaultPrincipalAccess()[1]}, nil
		}

		result, err := s.service.AllowVaultAccess(cli.AllowVaultAccessConfig{
			Vault: "deploys", ServiceAccount: "deploy-bot", AllowFor: "8h",
		})

		require.NoError(t, err)
		require.Equal(t, "service_account", result.AccessType)
		require.Contains(t, s.mockStdout.String(), `Allowed service account "deploy-bot"`)
	})

	t.Run("rejects conflicting subjects and expiration controls", func(t *testing.T) {
		s := setupTest(t)

		_, err := s.service.AllowVaultAccess(cli.AllowVaultAccessConfig{Email: "person@example.com", ServiceAccount: "deploy-bot"})
		require.ErrorContains(t, err, "exactly one")

		_, err = s.service.AllowVaultAccess(cli.AllowVaultAccessConfig{Email: "person@example.com", AllowFor: "8h", ExpiresAt: "2026-10-31T17:00:00Z"})
		require.ErrorContains(t, err, "mutually exclusive")
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

	t.Run("requires confirmation before revoking a user", func(t *testing.T) {
		s := setupTest(t)
		configureVaultAccessResolution(s)
		s.mockAPI.MockListVaultAccessGrants = func(cfg api.ListVaultAccessGrantsConfig) (*api.ListVaultAccessGrantsResult, error) {
			return &api.ListVaultAccessGrantsResult{AccessGrants: testVaultPrincipalAccess()}, nil
		}

		_, err := s.service.RevokeVaultAccess(cli.RevokeVaultAccessConfig{Vault: "deploys", Email: "person@example.com"})

		require.ErrorContains(t, err, "use --yes to confirm")
	})

	t.Run("revokes a service account by stable grant ID", func(t *testing.T) {
		s := setupTest(t)
		configureVaultAccessResolution(s)
		s.mockAPI.MockListVaultAccessGrants = func(cfg api.ListVaultAccessGrantsConfig) (*api.ListVaultAccessGrantsResult, error) {
			return &api.ListVaultAccessGrantsResult{AccessGrants: testVaultPrincipalAccess()}, nil
		}
		s.mockAPI.MockDeleteVaultAccessGrant = func(cfg api.DeleteVaultAccessGrantConfig) (*api.DeleteVaultAccessGrantResult, error) {
			require.Equal(t, "vault-1", cfg.VaultID)
			require.Equal(t, "identity-2", cfg.GrantID)
			return &api.DeleteVaultAccessGrantResult{}, nil
		}

		result, err := s.service.RevokeVaultAccess(cli.RevokeVaultAccessConfig{
			Vault: "deploys", ServiceAccount: "deploy-bot", Yes: true, Json: true,
		})

		require.NoError(t, err)
		require.Equal(t, "identity-2", result.ID)
		require.Contains(t, s.mockStdout.String(), `"ServiceAccountName":"deploy-bot"`)
	})
}
