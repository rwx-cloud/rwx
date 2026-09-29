package cli_test

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestService_CreateVaultOidcToken(t *testing.T) {
	t.Run("creates token with explicit name and audience", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockCreateVaultOidcToken = func(cfg api.CreateVaultOidcTokenConfig) (*api.CreateVaultOidcTokenResult, error) {
			require.Equal(t, "my-vault", cfg.VaultName)
			require.Equal(t, "my-token", cfg.Name)
			require.Equal(t, "sts.amazonaws.com", cfg.Audience)
			require.Empty(t, cfg.Provider)
			return &api.CreateVaultOidcTokenResult{
				Audience:         "sts.amazonaws.com",
				Subject:          "org:my-org:vault:my-vault",
				Expression:       "${{ vaults.my-vault.oidc_token.my-token }}",
				DocumentationURL: "https://www.rwx.com/docs/oidc",
			}, nil
		}

		result, err := s.service.CreateVaultOidcToken(cli.CreateVaultOidcTokenConfig{
			Vault:    "my-vault",
			Name:     "my-token",
			Audience: "sts.amazonaws.com",
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Contains(t, s.mockStdout.String(), `Created OIDC token "my-token" in vault "my-vault".`)
		require.Contains(t, s.mockStdout.String(), "Audience:   sts.amazonaws.com")
		require.Contains(t, s.mockStdout.String(), "Subject:    org:my-org:vault:my-vault")
		require.Contains(t, s.mockStdout.String(), "Expression: ${{ vaults.my-vault.oidc_token.my-token }}")
		require.Contains(t, s.mockStdout.String(), "For more information on configuring your identity provider and using your OIDC token, see: https://www.rwx.com/docs/oidc")
	})

	t.Run("passes provider through to the API", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockCreateVaultOidcToken = func(cfg api.CreateVaultOidcTokenConfig) (*api.CreateVaultOidcTokenResult, error) {
			require.Equal(t, "my-vault", cfg.VaultName)
			require.Equal(t, "aws", cfg.Provider)
			require.Empty(t, cfg.Name)
			require.Empty(t, cfg.Audience)
			return &api.CreateVaultOidcTokenResult{
				Audience:         "sts.amazonaws.com",
				Subject:          "org:my-org:vault:my-vault",
				Expression:       "${{ vaults.my-vault.oidc_token.aws }}",
				DocumentationURL: "https://www.rwx.com/docs/oidc-aws",
			}, nil
		}

		result, err := s.service.CreateVaultOidcToken(cli.CreateVaultOidcTokenConfig{
			Vault:    "my-vault",
			Provider: "aws",
		})

		require.NoError(t, err)
		require.NotNil(t, result)
	})

	t.Run("passes provider with explicit overrides to the API", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockCreateVaultOidcToken = func(cfg api.CreateVaultOidcTokenConfig) (*api.CreateVaultOidcTokenResult, error) {
			require.Equal(t, "aws", cfg.Provider)
			require.Equal(t, "custom", cfg.Name)
			require.Equal(t, "custom-aud", cfg.Audience)
			return &api.CreateVaultOidcTokenResult{
				Audience:         "custom-aud",
				Subject:          "org:my-org:vault:my-vault",
				Expression:       "${{ vaults.my-vault.oidc_token.custom }}",
				DocumentationURL: "https://www.rwx.com/docs/oidc-aws",
			}, nil
		}

		result, err := s.service.CreateVaultOidcToken(cli.CreateVaultOidcTokenConfig{
			Vault:    "my-vault",
			Provider: "aws",
			Name:     "custom",
			Audience: "custom-aud",
		})

		require.NoError(t, err)
		require.NotNil(t, result)
	})

	t.Run("errors when name is missing without provider", func(t *testing.T) {
		s := setupTest(t)

		result, err := s.service.CreateVaultOidcToken(cli.CreateVaultOidcTokenConfig{
			Vault:    "my-vault",
			Audience: "sts.amazonaws.com",
		})

		require.Nil(t, result)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--name is required")
	})

	t.Run("errors when audience is missing without provider", func(t *testing.T) {
		s := setupTest(t)

		result, err := s.service.CreateVaultOidcToken(cli.CreateVaultOidcTokenConfig{
			Vault: "my-vault",
			Name:  "my-token",
		})

		require.Nil(t, result)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--audience is required")
	})

	t.Run("when API returns an error", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockCreateVaultOidcToken = func(cfg api.CreateVaultOidcTokenConfig) (*api.CreateVaultOidcTokenResult, error) {
			return nil, errors.New("vault not found")
		}

		result, err := s.service.CreateVaultOidcToken(cli.CreateVaultOidcTokenConfig{
			Vault:    "my-vault",
			Name:     "my-token",
			Audience: "sts.amazonaws.com",
		})

		require.Nil(t, result)
		require.Error(t, err)
		require.Contains(t, err.Error(), "vault not found")
	})

	t.Run("with json output", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockCreateVaultOidcToken = func(cfg api.CreateVaultOidcTokenConfig) (*api.CreateVaultOidcTokenResult, error) {
			return &api.CreateVaultOidcTokenResult{
				Audience:         "sts.amazonaws.com",
				Subject:          "org:my-org:vault:my-vault",
				Expression:       "${{ vaults.my-vault.oidc_token.my-token }}",
				DocumentationURL: "https://www.rwx.com/docs/oidc",
			}, nil
		}

		result, err := s.service.CreateVaultOidcToken(cli.CreateVaultOidcTokenConfig{
			Vault:    "my-vault",
			Name:     "my-token",
			Audience: "sts.amazonaws.com",
			Json:     true,
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Contains(t, s.mockStdout.String(), `"Audience":"sts.amazonaws.com"`)
		require.Contains(t, s.mockStdout.String(), `"Subject":"org:my-org:vault:my-vault"`)
		require.Contains(t, s.mockStdout.String(), `"Expression":"${{ vaults.my-vault.oidc_token.my-token }}"`)
		require.Contains(t, s.mockStdout.String(), `"DocumentationURL":"https://www.rwx.com/docs/oidc"`)
	})
}

func testVaultOidcToken() api.VaultOidcToken {
	return api.VaultOidcToken{
		ID:               "token-1",
		Vault:            api.VaultIdentity{ID: "vault-1", Name: "deploys"},
		Name:             "aws",
		Audience:         "sts.amazonaws.com",
		Subject:          "org:acme:vault:deploys",
		Expression:       "${{ vaults.deploys.oidc.aws }}",
		DocumentationURL: "https://www.rwx.com/docs/oidc-aws",
	}
}

func configureVaultOidcTokenResolution(s *testSetup) {
	s.mockAPI.MockListVaults = func() (*api.ListVaultsResult, error) {
		return &api.ListVaultsResult{Vaults: []api.Vault{{ID: "vault-1", Name: "deploys"}}}, nil
	}
	s.mockAPI.MockListVaultOidcTokens = func(cfg api.ListVaultOidcTokensConfig) (*api.ListVaultOidcTokensResult, error) {
		return &api.ListVaultOidcTokensResult{OidcTokens: []api.VaultOidcToken{testVaultOidcToken()}}, nil
	}
}

func TestService_ListVaultOidcTokens(t *testing.T) {
	t.Run("resolves a vault name and prints complete JSON state", func(t *testing.T) {
		s := setupTest(t)
		s.mockAPI.MockListVaults = func() (*api.ListVaultsResult, error) {
			return &api.ListVaultsResult{Vaults: []api.Vault{{ID: "vault-1", Name: "deploys"}}}, nil
		}
		s.mockAPI.MockListVaultOidcTokens = func(cfg api.ListVaultOidcTokensConfig) (*api.ListVaultOidcTokensResult, error) {
			require.Equal(t, "vault-1", cfg.VaultID)
			return &api.ListVaultOidcTokensResult{OidcTokens: []api.VaultOidcToken{testVaultOidcToken()}}, nil
		}

		result, err := s.service.ListVaultOidcTokens(cli.ListVaultOidcTokensConfig{Vault: "deploys", Json: true})

		require.NoError(t, err)
		require.Len(t, result.OidcTokens, 1)
		require.JSONEq(t, `{"OidcTokens":[{"ID":"token-1","Vault":{"ID":"vault-1","Name":"deploys"},"Name":"aws","Audience":"sts.amazonaws.com","Subject":"org:acme:vault:deploys","Expression":"${{ vaults.deploys.oidc.aws }}","DocumentationURL":"https://www.rwx.com/docs/oidc-aws"}]}`, s.mockStdout.String())
	})

	t.Run("prints an empty message", func(t *testing.T) {
		s := setupTest(t)
		s.mockAPI.MockListVaults = func() (*api.ListVaultsResult, error) {
			return &api.ListVaultsResult{Vaults: []api.Vault{{ID: "vault-1", Name: "deploys"}}}, nil
		}
		s.mockAPI.MockListVaultOidcTokens = func(cfg api.ListVaultOidcTokensConfig) (*api.ListVaultOidcTokensResult, error) {
			return &api.ListVaultOidcTokensResult{}, nil
		}

		_, err := s.service.ListVaultOidcTokens(cli.ListVaultOidcTokensConfig{Vault: "deploys"})

		require.NoError(t, err)
		require.Equal(t, "No OIDC tokens found in vault \"deploys\".\n", s.mockStdout.String())
	})
}

func TestService_ShowVaultOidcToken(t *testing.T) {
	s := setupTest(t)
	configureVaultOidcTokenResolution(s)
	s.mockAPI.MockShowVaultOidcToken = func(cfg api.ShowVaultOidcTokenConfig) (*api.ShowVaultOidcTokenResult, error) {
		require.Equal(t, "vault-1", cfg.VaultID)
		require.Equal(t, "token-1", cfg.TokenID)
		result := testVaultOidcToken()
		return &result, nil
	}

	result, err := s.service.ShowVaultOidcToken(cli.VaultOidcTokenConfig{Vault: "deploys", Token: "aws"})

	require.NoError(t, err)
	require.Equal(t, "token-1", result.ID)
	require.Contains(t, s.mockStdout.String(), "ID: token-1")
	require.Contains(t, s.mockStdout.String(), "Vault: deploys (vault-1)")
	require.Contains(t, s.mockStdout.String(), "Documentation: https://www.rwx.com/docs/oidc-aws")
}

func TestService_UpdateVaultOidcToken(t *testing.T) {
	t.Run("resolves names and updates name and audience", func(t *testing.T) {
		s := setupTest(t)
		configureVaultOidcTokenResolution(s)
		s.mockAPI.MockUpdateVaultOidcToken = func(cfg api.UpdateVaultOidcTokenConfig) (*api.UpdateVaultOidcTokenResult, error) {
			require.Equal(t, "vault-1", cfg.VaultID)
			require.Equal(t, "token-1", cfg.TokenID)
			require.Equal(t, "production", cfg.Name)
			require.Equal(t, "https://example.com", cfg.Audience)
			result := testVaultOidcToken()
			result.Name = cfg.Name
			result.Audience = cfg.Audience
			return &result, nil
		}

		result, err := s.service.UpdateVaultOidcToken(cli.UpdateVaultOidcTokenConfig{
			Vault: "deploys", Token: "aws", Name: "production", Audience: "https://example.com", Json: true,
		})

		require.NoError(t, err)
		require.Equal(t, "production", result.Name)
		require.Contains(t, s.mockStdout.String(), `"ID":"token-1"`)
		require.Contains(t, s.mockStdout.String(), `"Audience":"https://example.com"`)
	})

	t.Run("requires at least one change", func(t *testing.T) {
		s := setupTest(t)
		_, err := s.service.UpdateVaultOidcToken(cli.UpdateVaultOidcTokenConfig{Vault: "deploys", Token: "aws"})
		require.ErrorContains(t, err, "provide --name, --audience, or both")
	})
}

func TestService_DeleteVaultOidcToken(t *testing.T) {
	t.Run("requires confirmation in non-interactive environments", func(t *testing.T) {
		s := setupTest(t)
		configureVaultOidcTokenResolution(s)

		_, err := s.service.DeleteVaultOidcToken(cli.DeleteVaultOidcTokenConfig{Vault: "deploys", Token: "aws"})

		require.ErrorContains(t, err, "use --yes to confirm")
	})

	t.Run("deletes the resolved token and returns IDs", func(t *testing.T) {
		s := setupTest(t)
		configureVaultOidcTokenResolution(s)
		s.mockAPI.MockDeleteVaultOidcToken = func(cfg api.DeleteVaultOidcTokenConfig) (*api.DeleteVaultOidcTokenResult, error) {
			require.Equal(t, "vault-1", cfg.VaultID)
			require.Equal(t, "token-1", cfg.TokenID)
			return &api.DeleteVaultOidcTokenResult{}, nil
		}

		result, err := s.service.DeleteVaultOidcToken(cli.DeleteVaultOidcTokenConfig{Vault: "deploys", Token: "aws", Json: true, Yes: true})

		require.NoError(t, err)
		require.Equal(t, "token-1", result.ID)
		require.JSONEq(t, `{"ID":"token-1","Vault":{"ID":"vault-1","Name":"deploys"},"Name":"aws"}`, s.mockStdout.String())
	})
}
