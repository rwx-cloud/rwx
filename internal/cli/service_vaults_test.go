package cli_test

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestService_CreateVault(t *testing.T) {
	t.Run("when unable to create vault", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockCreateVault = func(cfg api.CreateVaultConfig) (*api.CreateVaultResult, error) {
			require.Equal(t, "my-vault", cfg.Name)
			require.False(t, cfg.Unlocked)
			return nil, errors.New("vault already exists")
		}

		result, err := s.service.CreateVault(cli.CreateVaultConfig{
			Name: "my-vault",
		})

		require.Nil(t, result)
		require.Error(t, err)
		require.Contains(t, err.Error(), "vault already exists")
	})

	t.Run("creates a vault successfully", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockCreateVault = func(cfg api.CreateVaultConfig) (*api.CreateVaultResult, error) {
			require.Equal(t, "my-vault", cfg.Name)
			require.False(t, cfg.Unlocked)
			require.Empty(t, cfg.RepositoryPermissions)
			return &api.CreateVaultResult{}, nil
		}

		result, err := s.service.CreateVault(cli.CreateVaultConfig{
			Name: "my-vault",
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "Created vault \"my-vault\".\n", s.mockStdout.String())
	})

	t.Run("creates a vault with repository permissions", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockCreateVault = func(cfg api.CreateVaultConfig) (*api.CreateVaultResult, error) {
			require.Equal(t, "my-vault", cfg.Name)
			require.True(t, cfg.Unlocked)
			require.Len(t, cfg.RepositoryPermissions, 3)
			require.Equal(t, "my-repo", cfg.RepositoryPermissions[0].RepositorySlug)
			require.Equal(t, "main", cfg.RepositoryPermissions[0].BranchPattern)
			require.Equal(t, "other-repo", cfg.RepositoryPermissions[1].RepositorySlug)
			require.Equal(t, "refs/heads/release/*", cfg.RepositoryPermissions[1].BranchPattern)
			require.Equal(t, "tagged-repo", cfg.RepositoryPermissions[2].RepositorySlug)
			require.Equal(t, "refs/tags/v*", cfg.RepositoryPermissions[2].BranchPattern)
			return &api.CreateVaultResult{}, nil
		}

		result, err := s.service.CreateVault(cli.CreateVaultConfig{
			Name:                  "my-vault",
			Unlocked:              true,
			RepositoryPermissions: []string{"my-repo:main", "other-repo:refs/heads/release/*", "tagged-repo:refs/tags/v*"},
		})

		require.NoError(t, err)
		require.NotNil(t, result)
	})

	t.Run("validates repository permission format", func(t *testing.T) {
		s := setupTest(t)

		result, err := s.service.CreateVault(cli.CreateVaultConfig{
			Name:                  "my-vault",
			RepositoryPermissions: []string{"bad-format"},
		})

		require.Nil(t, result)
		require.Error(t, err)
		require.Contains(t, err.Error(), "REPO_SLUG:REF_PATTERN")
	})

	t.Run("validates name is required", func(t *testing.T) {
		s := setupTest(t)

		result, err := s.service.CreateVault(cli.CreateVaultConfig{})

		require.Nil(t, result)
		require.Error(t, err)
		require.Contains(t, err.Error(), "vault name must be provided")
	})

	t.Run("with json output", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockCreateVault = func(cfg api.CreateVaultConfig) (*api.CreateVaultResult, error) {
			return &api.CreateVaultResult{Vault: api.Vault{
				ID:          "vault-1",
				Name:        "my-vault",
				LockStatus:  "locked",
				OidcSubject: "org:acme:vault:my-vault",
			}}, nil
		}

		result, err := s.service.CreateVault(cli.CreateVaultConfig{
			Name: "my-vault",
			Json: true,
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		require.JSONEq(t, `{"Vault":"my-vault","ID":"vault-1","Name":"my-vault","LockStatus":"locked","RepositoryPermissions":[],"OidcSubject":"org:acme:vault:my-vault"}`, s.mockStdout.String())
	})
}

func TestService_ListVaults(t *testing.T) {
	t.Run("prints vault details", func(t *testing.T) {
		s := setupTest(t)
		s.mockAPI.MockListVaults = func() (*api.ListVaultsResult, error) {
			return &api.ListVaultsResult{Vaults: []api.Vault{
				{Name: "default", LockStatus: "unlocked"},
				{
					Name:       "deploys",
					LockStatus: "locked",
					RepositoryPermissions: []api.CreateVaultRepoPermission{
						{RepositorySlug: "rwx-cloud/cloud", BranchPattern: "main"},
						{RepositorySlug: "rwx-cloud/zebra", BranchPattern: "release/**"},
					},
				},
			}}, nil
		}

		result, err := s.service.ListVaults(cli.ListVaultsConfig{})
		require.NoError(t, err)
		require.Len(t, result.Vaults, 2)
		require.Equal(t, "NAME     LOCK STATUS  REPOSITORY PERMISSIONS\ndefault  unlocked     \ndeploys  locked       rwx-cloud/cloud:main, rwx-cloud/zebra:release/**\n", s.mockStdout.String())
	})

	t.Run("uses PascalCase JSON fields", func(t *testing.T) {
		s := setupTest(t)
		s.mockAPI.MockListVaults = func() (*api.ListVaultsResult, error) {
			return &api.ListVaultsResult{Vaults: []api.Vault{{
				ID:          "vault-1",
				Name:        "deploys",
				LockStatus:  "locked",
				OidcSubject: "org:acme:vault:deploys",
				RepositoryPermissions: []api.CreateVaultRepoPermission{
					{RepositorySlug: "rwx-cloud/cloud", BranchPattern: "main"},
				},
			}}}, nil
		}

		_, err := s.service.ListVaults(cli.ListVaultsConfig{Json: true})
		require.NoError(t, err)
		require.JSONEq(t, `{"Vaults":[{"ID":"vault-1","Name":"deploys","LockStatus":"locked","RepositoryPermissions":[{"RepositorySlug":"rwx-cloud/cloud","BranchPattern":"main"}],"OidcSubject":"org:acme:vault:deploys"}]}`, s.mockStdout.String())
	})
}

func testVault() api.Vault {
	return api.Vault{
		ID:          "vault-1",
		Name:        "deploys",
		LockStatus:  "locked",
		OidcSubject: "org:acme:vault:deploys",
		RepositoryPermissions: []api.CreateVaultRepoPermission{
			{RepositorySlug: "rwx-cloud/cloud", BranchPattern: "main"},
		},
	}
}

func configureVaultResolution(s *testSetup) {
	s.mockAPI.MockListVaults = func() (*api.ListVaultsResult, error) {
		return &api.ListVaultsResult{Vaults: []api.Vault{testVault()}}, nil
	}
}

func TestService_ShowVault(t *testing.T) {
	t.Run("prints complete JSON", func(t *testing.T) {
		s := setupTest(t)
		configureVaultResolution(s)
		s.mockAPI.MockShowVault = func(cfg api.ShowVaultConfig) (*api.ShowVaultResult, error) {
			require.Equal(t, "vault-1", cfg.VaultID)
			return &api.ShowVaultResult{Vault: testVault()}, nil
		}

		result, err := s.service.ShowVault(cli.ShowVaultConfig{Vault: "deploys", Json: true})

		require.NoError(t, err)
		require.Equal(t, "vault-1", result.ID)
		require.JSONEq(t, `{"ID":"vault-1","Name":"deploys","LockStatus":"locked","RepositoryPermissions":[{"RepositorySlug":"rwx-cloud/cloud","BranchPattern":"main"}],"OidcSubject":"org:acme:vault:deploys"}`, s.mockStdout.String())
	})

	t.Run("prints complete text", func(t *testing.T) {
		s := setupTest(t)
		configureVaultResolution(s)
		s.mockAPI.MockShowVault = func(cfg api.ShowVaultConfig) (*api.ShowVaultResult, error) {
			return &api.ShowVaultResult{Vault: testVault()}, nil
		}

		_, err := s.service.ShowVault(cli.ShowVaultConfig{Vault: "deploys"})

		require.NoError(t, err)
		require.Equal(t, "ID: vault-1\nName: deploys\nLock status: locked\nRepository permissions: rwx-cloud/cloud:main\nOIDC subject: org:acme:vault:deploys\n", s.mockStdout.String())
	})
}

func TestService_UpdateVault(t *testing.T) {
	t.Run("updates name and lock state and prints complete state", func(t *testing.T) {
		s := setupTest(t)
		configureVaultResolution(s)
		s.mockAPI.MockUpdateVault = func(cfg api.UpdateVaultConfig) (*api.UpdateVaultResult, error) {
			require.Equal(t, "vault-1", cfg.VaultID)
			require.Equal(t, "production", *cfg.Name)
			require.False(t, *cfg.Unlocked)
			vault := testVault()
			vault.Name = "production"
			return &api.UpdateVaultResult{Vault: vault}, nil
		}

		result, err := s.service.UpdateVault(cli.UpdateVaultConfig{
			Vault:       "deploys",
			Name:        "production",
			NameSet:     true,
			Unlocked:    false,
			UnlockedSet: true,
			Json:        true,
		})

		require.NoError(t, err)
		require.Equal(t, "production", result.Name)
		require.Contains(t, s.mockStdout.String(), `"ID":"vault-1"`)
		require.Contains(t, s.mockStdout.String(), `"OidcSubject":"org:acme:vault:deploys"`)
	})

	t.Run("requires at least one change", func(t *testing.T) {
		s := setupTest(t)
		_, err := s.service.UpdateVault(cli.UpdateVaultConfig{Vault: "deploys"})
		require.ErrorContains(t, err, "provide --name, --unlocked, or both")
	})

	t.Run("prints updated text", func(t *testing.T) {
		s := setupTest(t)
		configureVaultResolution(s)
		s.mockAPI.MockUpdateVault = func(cfg api.UpdateVaultConfig) (*api.UpdateVaultResult, error) {
			vault := testVault()
			vault.Name = *cfg.Name
			return &api.UpdateVaultResult{Vault: vault}, nil
		}

		_, err := s.service.UpdateVault(cli.UpdateVaultConfig{Vault: "deploys", Name: "production", NameSet: true})

		require.NoError(t, err)
		require.Contains(t, s.mockStdout.String(), "Updated vault \"production\".")
		require.Contains(t, s.mockStdout.String(), "ID: vault-1")
	})
}

func TestService_DeleteVault(t *testing.T) {
	t.Run("requires confirmation in non-interactive environments", func(t *testing.T) {
		s := setupTest(t)
		configureVaultResolution(s)
		s.mockAPI.MockShowVault = func(cfg api.ShowVaultConfig) (*api.ShowVaultResult, error) {
			return &api.ShowVaultResult{Vault: testVault()}, nil
		}

		_, err := s.service.DeleteVault(cli.DeleteVaultConfig{Vault: "deploys"})

		require.ErrorContains(t, err, "use --yes to confirm")
	})

	t.Run("deletes the resolved vault and returns complete JSON", func(t *testing.T) {
		s := setupTest(t)
		configureVaultResolution(s)
		s.mockAPI.MockShowVault = func(cfg api.ShowVaultConfig) (*api.ShowVaultResult, error) {
			return &api.ShowVaultResult{Vault: testVault()}, nil
		}
		s.mockAPI.MockDeleteVault = func(cfg api.DeleteVaultConfig) (*api.DeleteVaultResult, error) {
			require.Equal(t, "vault-1", cfg.VaultID)
			return &api.DeleteVaultResult{}, nil
		}

		result, err := s.service.DeleteVault(cli.DeleteVaultConfig{Vault: "deploys", Json: true, Yes: true})

		require.NoError(t, err)
		require.Equal(t, "vault-1", result.ID)
		require.JSONEq(t, `{"ID":"vault-1","Name":"deploys","LockStatus":"locked","RepositoryPermissions":[{"RepositorySlug":"rwx-cloud/cloud","BranchPattern":"main"}],"OidcSubject":"org:acme:vault:deploys"}`, s.mockStdout.String())
	})

	t.Run("surfaces protected vault errors", func(t *testing.T) {
		s := setupTest(t)
		configureVaultResolution(s)
		s.mockAPI.MockShowVault = func(cfg api.ShowVaultConfig) (*api.ShowVaultResult, error) {
			return &api.ShowVaultResult{Vault: testVault()}, nil
		}
		s.mockAPI.MockDeleteVault = func(cfg api.DeleteVaultConfig) (*api.DeleteVaultResult, error) {
			return nil, errors.New("The default vault cannot be deleted.")
		}

		_, err := s.service.DeleteVault(cli.DeleteVaultConfig{Vault: "deploys", Yes: true})

		require.ErrorContains(t, err, "default vault cannot be deleted")
	})

	t.Run("prints deletion text", func(t *testing.T) {
		s := setupTest(t)
		configureVaultResolution(s)
		s.mockAPI.MockShowVault = func(cfg api.ShowVaultConfig) (*api.ShowVaultResult, error) {
			return &api.ShowVaultResult{Vault: testVault()}, nil
		}
		s.mockAPI.MockDeleteVault = func(cfg api.DeleteVaultConfig) (*api.DeleteVaultResult, error) {
			return &api.DeleteVaultResult{}, nil
		}

		_, err := s.service.DeleteVault(cli.DeleteVaultConfig{Vault: "deploys", Yes: true})

		require.NoError(t, err)
		require.Equal(t, "Deleted vault \"deploys\".\n", s.mockStdout.String())
	})
}
