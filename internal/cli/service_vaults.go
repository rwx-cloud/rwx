package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type CreateVaultConfig struct {
	Name                  string
	Unlocked              bool
	RepositoryPermissions []string
	Json                  bool
}

func (c CreateVaultConfig) Validate() error {
	if c.Name == "" {
		return errors.New("the vault name must be provided")
	}

	for _, rp := range c.RepositoryPermissions {
		if !strings.Contains(rp, ":") {
			return errors.New(fmt.Sprintf("invalid repository permission %q: must be in the format REPO_SLUG:REF_PATTERN", rp))
		}
	}

	return nil
}

type CreateVaultResult struct{}

func (s Service) CreateVault(cfg CreateVaultConfig) (*CreateVaultResult, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	repoPermissions := []api.CreateVaultRepoPermission{}
	for _, rp := range cfg.RepositoryPermissions {
		slug, pattern, _ := strings.Cut(rp, ":")
		repoPermissions = append(repoPermissions, api.CreateVaultRepoPermission{
			RepositorySlug: slug,
			BranchPattern:  pattern,
		})
	}

	_, err = s.APIClient.CreateVault(api.CreateVaultConfig{
		Name:                  cfg.Name,
		Unlocked:              cfg.Unlocked,
		RepositoryPermissions: repoPermissions,
	})

	if err != nil {
		return nil, errors.Wrap(err, "unable to create vault")
	}

	if cfg.Json {
		output := struct {
			Vault string
		}{
			Vault: cfg.Name,
		}
		if err := json.NewEncoder(s.Stdout).Encode(output); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		fmt.Fprintf(s.Stdout, "Created vault %q.\n", cfg.Name)
	}

	return &CreateVaultResult{}, nil
}
