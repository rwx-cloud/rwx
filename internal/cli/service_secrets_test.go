package cli_test

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/pkg/errors"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestService_SettingSecrets(t *testing.T) {
	t.Run("when unable to set secrets", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockSetSecretsInVault = func(ssivc api.SetSecretsInVaultConfig) (*api.SetSecretsInVaultResult, error) {
			require.Equal(t, "default", ssivc.VaultName)
			require.Equal(t, "ABC", ssivc.Secrets[0].Name)
			require.Equal(t, "123", ssivc.Secrets[0].Secret)
			return nil, errors.New("error setting secret")
		}

		result, err := s.service.SetSecretsInVault(cli.SetSecretsInVaultConfig{
			Vault:   "default",
			Secrets: []string{"ABC=123"},
		})

		require.Nil(t, result)
		require.Error(t, err)
		require.Contains(t, err.Error(), "error setting secret")
	})

	t.Run("with secrets set", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockSetSecretsInVault = func(ssivc api.SetSecretsInVaultConfig) (*api.SetSecretsInVaultResult, error) {
			require.Equal(t, "default", ssivc.VaultName)
			require.Equal(t, "ABC", ssivc.Secrets[0].Name)
			require.Equal(t, "123", ssivc.Secrets[0].Secret)
			require.Equal(t, "DEF", ssivc.Secrets[1].Name)
			require.Equal(t, `"xyz"`, ssivc.Secrets[1].Secret)
			return &api.SetSecretsInVaultResult{
				SetSecrets: []string{"ABC", "DEF"},
			}, nil
		}

		result, err := s.service.SetSecretsInVault(cli.SetSecretsInVaultConfig{
			Vault:   "default",
			Secrets: []string{"ABC=123", `DEF="xyz"`},
		})

		require.NoError(t, err)
		require.Equal(t, []string{"ABC", "DEF"}, result.SetSecrets)
		require.Equal(t, "Successfully set the following secrets: ABC, DEF\n", s.mockStdout.String())
	})

	t.Run("when reading secrets from a file", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockSetSecretsInVault = func(ssivc api.SetSecretsInVaultConfig) (*api.SetSecretsInVaultResult, error) {
			sort.Slice(ssivc.Secrets, func(i, j int) bool {
				return ssivc.Secrets[i].Name < ssivc.Secrets[j].Name
			})
			require.Equal(t, "default", ssivc.VaultName)
			require.Equal(t, "A", ssivc.Secrets[0].Name)
			require.Equal(t, "123", ssivc.Secrets[0].Secret)
			require.Equal(t, "B", ssivc.Secrets[1].Name)
			require.Equal(t, "xyz", ssivc.Secrets[1].Secret)
			require.Equal(t, "C", ssivc.Secrets[2].Name)
			require.Equal(t, "q\\nqq", ssivc.Secrets[2].Secret)
			require.Equal(t, "D", ssivc.Secrets[3].Name)
			require.Equal(t, "a multiline\nstring\nspanning lines", ssivc.Secrets[3].Secret)
			return &api.SetSecretsInVaultResult{
				SetSecrets: []string{"A", "B", "C", "D"},
			}, nil
		}

		secretsFile := filepath.Join(s.tmp, "secrets.txt")
		err := os.WriteFile(secretsFile, []byte("A=123\nB=\"xyz\"\nC='q\\nqq'\nD=\"a multiline\nstring\nspanning lines\""), 0o644)
		require.NoError(t, err)

		result, err := s.service.SetSecretsInVault(cli.SetSecretsInVaultConfig{
			Vault:   "default",
			Secrets: []string{},
			File:    secretsFile,
		})

		require.NoError(t, err)
		require.Equal(t, []string{"A", "B", "C", "D"}, result.SetSecrets)
		require.Equal(t, "Successfully set the following secrets: A, B, C, D\n", s.mockStdout.String())
	})
}

func TestService_DeleteSecret(t *testing.T) {
	t.Run("deletes a secret", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockDeleteSecret = func(cfg api.DeleteSecretConfig) (*api.DeleteSecretResult, error) {
			require.Equal(t, "MY_SECRET", cfg.SecretName)
			require.Equal(t, "default", cfg.VaultName)
			return &api.DeleteSecretResult{}, nil
		}

		result, err := s.service.DeleteSecret(cli.DeleteSecretConfig{
			SecretName: "MY_SECRET",
			Vault:      "default",
			Yes:        true,
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "Deleted secret \"MY_SECRET\" from vault \"default\".\n", s.mockStdout.String())
	})

	t.Run("when unable to delete secret", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockDeleteSecret = func(cfg api.DeleteSecretConfig) (*api.DeleteSecretResult, error) {
			return nil, errors.New("not found")
		}

		result, err := s.service.DeleteSecret(cli.DeleteSecretConfig{
			SecretName: "MISSING",
			Vault:      "default",
			Yes:        true,
		})

		require.Nil(t, result)
		require.Error(t, err)
		require.Contains(t, err.Error(), "not found")
	})

	t.Run("with json output", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockDeleteSecret = func(cfg api.DeleteSecretConfig) (*api.DeleteSecretResult, error) {
			return &api.DeleteSecretResult{}, nil
		}

		result, err := s.service.DeleteSecret(cli.DeleteSecretConfig{
			SecretName: "MY_SECRET",
			Vault:      "default",
			Json:       true,
			Yes:        true,
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Contains(t, s.mockStdout.String(), `"Secret":"MY_SECRET"`)
		require.Contains(t, s.mockStdout.String(), `"Vault":"default"`)
	})

	t.Run("requires --yes in non-interactive environments", func(t *testing.T) {
		s := setupTest(t)

		_, err := s.service.DeleteSecret(cli.DeleteSecretConfig{
			SecretName: "MY_SECRET",
			Vault:      "default",
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "use --yes to confirm")
	})

	t.Run("prompts for confirmation in TTY", func(t *testing.T) {
		s := setupTestWithTTY(t)
		s.mockStdin.WriteString("y\n")

		s.mockAPI.MockDeleteSecret = func(cfg api.DeleteSecretConfig) (*api.DeleteSecretResult, error) {
			return &api.DeleteSecretResult{}, nil
		}

		result, err := s.service.DeleteSecret(cli.DeleteSecretConfig{
			SecretName: "MY_SECRET",
			Vault:      "default",
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Contains(t, s.mockStderr.String(), `Delete secret "MY_SECRET" from vault "default"?`)
	})

	t.Run("aborts when user declines confirmation", func(t *testing.T) {
		s := setupTestWithTTY(t)
		s.mockStdin.WriteString("n\n")

		_, err := s.service.DeleteSecret(cli.DeleteSecretConfig{
			SecretName: "MY_SECRET",
			Vault:      "default",
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "aborted")
	})
}
