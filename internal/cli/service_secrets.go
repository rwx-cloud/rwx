package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/dotenv"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type DeleteSecretConfig struct {
	SecretName string
	Vault      string
	Json       bool
	Yes        bool
}

func (c DeleteSecretConfig) Validate() error {
	if c.SecretName == "" {
		return errors.New("the secret name must be provided")
	}

	if c.Vault == "" {
		return errors.New("the vault name must be provided")
	}

	return nil
}

type DeleteSecretResult struct{}

func (s Service) DeleteSecret(cfg DeleteSecretConfig) (*DeleteSecretResult, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	if err := s.confirmDestruction(
		fmt.Sprintf("Delete secret %q from vault %q?", cfg.SecretName, cfg.Vault),
		cfg.Yes,
	); err != nil {
		return nil, err
	}

	_, err = s.APIClient.DeleteSecret(api.DeleteSecretConfig{
		SecretName: cfg.SecretName,
		VaultName:  cfg.Vault,
	})
	if err != nil {
		return nil, errors.Wrap(err, "unable to delete secret")
	}

	if cfg.Json {
		output := struct {
			Secret string
			Vault  string
		}{
			Secret: cfg.SecretName,
			Vault:  cfg.Vault,
		}
		if err := json.NewEncoder(s.Stdout).Encode(output); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		fmt.Fprintf(s.Stdout, "Deleted secret %q from vault %q.\n", cfg.SecretName, cfg.Vault)
	}

	return &DeleteSecretResult{}, nil
}

type SetSecretsInVaultConfig struct {
	Secrets []string
	Vault   string
	File    string
	Json    bool
}

func (c SetSecretsInVaultConfig) Validate() error {
	if c.Vault == "" {
		return errors.New("the vault name must be provided")
	}

	if len(c.Secrets) == 0 && c.File == "" {
		return errors.New("the secrets to set must be provided")
	}

	return nil
}

func (s Service) SetSecretsInVault(cfg SetSecretsInVaultConfig) (*api.SetSecretsInVaultResult, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	secrets := []api.Secret{}
	for i := range cfg.Secrets {
		key, value, found := strings.Cut(cfg.Secrets[i], "=")
		if !found {
			return nil, errors.New(fmt.Sprintf("Invalid secret '%s'. Secrets must be specified in the form 'KEY=value'.", cfg.Secrets[i]))
		}
		secrets = append(secrets, api.Secret{
			Name:   key,
			Secret: value,
		})
	}

	if cfg.File != "" {
		fd, err := os.Open(cfg.File)
		if err != nil {
			return nil, errors.Wrapf(err, "error while opening %q", cfg.File)
		}
		defer fd.Close()

		fileContent, err := io.ReadAll(fd)
		if err != nil {
			return nil, errors.Wrapf(err, "error while reading %q", cfg.File)
		}

		dotenvMap := make(map[string]string)
		err = dotenv.ParseBytes(fileContent, dotenvMap)
		if err != nil {
			return nil, errors.Wrapf(err, "error while parsing %q", cfg.File)
		}

		for key, value := range dotenvMap {
			secrets = append(secrets, api.Secret{
				Name:   key,
				Secret: value,
			})
		}
	}

	result, err := s.APIClient.SetSecretsInVault(api.SetSecretsInVaultConfig{
		VaultName: cfg.Vault,
		Secrets:   secrets,
	})

	if err != nil {
		return nil, errors.Wrap(err, "unable to set secrets")
	}

	if cfg.Json {
		output := struct {
			Vault      string
			SetSecrets []string
		}{
			Vault:      cfg.Vault,
			SetSecrets: result.SetSecrets,
		}
		if err := json.NewEncoder(s.Stdout).Encode(output); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else if result != nil && len(result.SetSecrets) > 0 {
		fmt.Fprintf(s.Stdout, "Successfully set the following secrets: %s\n", strings.Join(result.SetSecrets, ", "))
	}

	return result, nil
}
