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

type VaultInfo struct {
	Name                  string
	LockStatus            string
	RepositoryPermissions []RepositoryPermissionInfo
}

type RepositoryPermissionInfo struct {
	RepositorySlug string
	BranchPattern  string
}

type ListVaultsResult struct {
	Vaults []VaultInfo
}

type ListVaultsConfig struct {
	Json bool
}

func (s Service) ListVaults(cfg ListVaultsConfig) (*ListVaultsResult, error) {
	apiResult, err := s.APIClient.ListVaults()
	if err != nil {
		return nil, errors.Wrap(err, "unable to list vaults")
	}

	vaults := make([]VaultInfo, len(apiResult.Vaults))
	for i, vault := range apiResult.Vaults {
		permissions := make([]RepositoryPermissionInfo, len(vault.RepositoryPermissions))
		for j, permission := range vault.RepositoryPermissions {
			permissions[j] = RepositoryPermissionInfo{
				RepositorySlug: permission.RepositorySlug,
				BranchPattern:  permission.BranchPattern,
			}
		}
		vaults[i] = VaultInfo{
			Name:                  vault.Name,
			LockStatus:            vault.LockStatus,
			RepositoryPermissions: permissions,
		}
	}

	result := &ListVaultsResult{Vaults: vaults}
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else if len(vaults) == 0 {
		fmt.Fprintln(s.Stdout, "No vaults found.")
	} else {
		nameWidth := len("NAME")
		statusWidth := len("LOCK STATUS")
		for _, vault := range vaults {
			if len(vault.Name) > nameWidth {
				nameWidth = len(vault.Name)
			}
			if len(vault.LockStatus) > statusWidth {
				statusWidth = len(vault.LockStatus)
			}
		}
		format := fmt.Sprintf("%%-%ds  %%-%ds  %%s\n", nameWidth, statusWidth)
		fmt.Fprintf(s.Stdout, format, "NAME", "LOCK STATUS", "REPOSITORY PERMISSIONS")
		for _, vault := range vaults {
			permissions := make([]string, len(vault.RepositoryPermissions))
			for i, permission := range vault.RepositoryPermissions {
				permissions[i] = permission.RepositorySlug + ":" + permission.BranchPattern
			}
			fmt.Fprintf(s.Stdout, format, vault.Name, vault.LockStatus, strings.Join(permissions, ", "))
		}
	}

	return result, nil
}
