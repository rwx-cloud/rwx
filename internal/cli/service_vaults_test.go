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
			return &api.CreateVaultResult{}, nil
		}

		result, err := s.service.CreateVault(cli.CreateVaultConfig{
			Name: "my-vault",
			Json: true,
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Contains(t, s.mockStdout.String(), `"Vault":"my-vault"`)
	})
}
