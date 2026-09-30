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

type CreateVaultResult struct {
	Vault                 string
	ID                    string
	Name                  string
	LockStatus            string
	RepositoryPermissions []RepositoryPermissionInfo
	OidcSubject           string
}

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

	apiResult, err := s.APIClient.CreateVault(api.CreateVaultConfig{
		Name:                  cfg.Name,
		Unlocked:              cfg.Unlocked,
		RepositoryPermissions: repoPermissions,
	})

	if err != nil {
		return nil, errors.Wrap(err, "unable to create vault")
	}

	vault := vaultInfo(apiResult.Vault)
	if vault.Name == "" {
		vault.Name = cfg.Name
		if cfg.Unlocked {
			vault.LockStatus = "unlocked"
		} else {
			vault.LockStatus = "locked"
		}
		vault.RepositoryPermissions = make([]RepositoryPermissionInfo, len(repoPermissions))
		for i, permission := range repoPermissions {
			vault.RepositoryPermissions[i] = RepositoryPermissionInfo{
				RepositorySlug: permission.RepositorySlug,
				BranchPattern:  permission.BranchPattern,
			}
		}
	}
	result := &CreateVaultResult{
		Vault:                 cfg.Name,
		ID:                    vault.ID,
		Name:                  vault.Name,
		LockStatus:            vault.LockStatus,
		RepositoryPermissions: vault.RepositoryPermissions,
		OidcSubject:           vault.OidcSubject,
	}

	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		fmt.Fprintf(s.Stdout, "Created vault %q.\n", cfg.Name)
	}

	return result, nil
}

type VaultInfo struct {
	ID                    string
	Name                  string
	LockStatus            string
	RepositoryPermissions []RepositoryPermissionInfo
	OidcSubject           string
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

type ShowVaultConfig struct {
	Vault string
	Json  bool
}

type ShowVaultResult = VaultInfo

type UpdateVaultConfig struct {
	Vault       string
	Name        string
	NameSet     bool
	Unlocked    bool
	UnlockedSet bool
	Json        bool
}

type UpdateVaultResult = VaultInfo

type DeleteVaultConfig struct {
	Vault string
	Json  bool
	Yes   bool
}

type DeleteVaultResult = VaultInfo

func vaultInfo(vault api.Vault) VaultInfo {
	permissions := make([]RepositoryPermissionInfo, len(vault.RepositoryPermissions))
	for i, permission := range vault.RepositoryPermissions {
		permissions[i] = RepositoryPermissionInfo{
			RepositorySlug: permission.RepositorySlug,
			BranchPattern:  permission.BranchPattern,
		}
	}
	return VaultInfo{
		ID:                    vault.ID,
		Name:                  vault.Name,
		LockStatus:            vault.LockStatus,
		RepositoryPermissions: permissions,
		OidcSubject:           vault.OidcSubject,
	}
}

func writeVault(stdout interface{ Write([]byte) (int, error) }, vault VaultInfo) {
	permissions := make([]string, len(vault.RepositoryPermissions))
	for i, permission := range vault.RepositoryPermissions {
		permissions[i] = permission.RepositorySlug + ":" + permission.BranchPattern
	}
	fmt.Fprintf(stdout, "ID: %s\n", vault.ID)
	fmt.Fprintf(stdout, "Name: %s\n", vault.Name)
	fmt.Fprintf(stdout, "Lock status: %s\n", vault.LockStatus)
	fmt.Fprintf(stdout, "Repository permissions: %s\n", strings.Join(permissions, ", "))
	fmt.Fprintf(stdout, "OIDC subject: %s\n", vault.OidcSubject)
}

func (s Service) ListVaults(cfg ListVaultsConfig) (*ListVaultsResult, error) {
	apiResult, err := s.APIClient.ListVaults()
	if err != nil {
		return nil, errors.Wrap(err, "unable to list vaults")
	}

	vaults := make([]VaultInfo, len(apiResult.Vaults))
	for i, vault := range apiResult.Vaults {
		vaults[i] = vaultInfo(vault)
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

func (s Service) ShowVault(cfg ShowVaultConfig) (*ShowVaultResult, error) {
	vaultID, err := s.resolveVaultID(cfg.Vault)
	if err != nil {
		return nil, err
	}

	apiResult, err := s.APIClient.ShowVault(api.ShowVaultConfig{VaultID: vaultID})
	if err != nil {
		return nil, errors.Wrap(err, "unable to show vault")
	}
	result := vaultInfo(apiResult.Vault)
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		writeVault(s.Stdout, result)
	}

	return &result, nil
}

func (s Service) UpdateVault(cfg UpdateVaultConfig) (*UpdateVaultResult, error) {
	if !cfg.NameSet && !cfg.UnlockedSet {
		return nil, errors.New("provide --name, --unlocked, or both")
	}
	if cfg.NameSet && cfg.Name == "" {
		return nil, errors.New("the vault name must not be empty")
	}

	apiConfig := api.UpdateVaultConfig{}
	if cfg.NameSet {
		apiConfig.Name = &cfg.Name
	}
	if cfg.UnlockedSet {
		apiConfig.Unlocked = &cfg.Unlocked
	}

	vaultID, err := s.resolveVaultID(cfg.Vault)
	if err != nil {
		return nil, err
	}
	apiConfig.VaultID = vaultID
	apiResult, err := s.APIClient.UpdateVault(apiConfig)
	if err != nil {
		return nil, errors.Wrap(err, "unable to update vault")
	}
	result := vaultInfo(apiResult.Vault)
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		fmt.Fprintf(s.Stdout, "Updated vault %q.\n\n", result.Name)
		writeVault(s.Stdout, result)
	}

	return &result, nil
}

func (s Service) DeleteVault(cfg DeleteVaultConfig) (*DeleteVaultResult, error) {
	vaultID, err := s.resolveVaultID(cfg.Vault)
	if err != nil {
		return nil, err
	}
	apiResult, err := s.APIClient.ShowVault(api.ShowVaultConfig{VaultID: vaultID})
	if err != nil {
		return nil, errors.Wrap(err, "unable to show vault")
	}
	vault := vaultInfo(apiResult.Vault)

	if err := s.confirmDestruction(fmt.Sprintf("Delete vault %q?", vault.Name), cfg.Yes); err != nil {
		return nil, err
	}
	if _, err := s.APIClient.DeleteVault(api.DeleteVaultConfig{VaultID: vault.ID}); err != nil {
		return nil, errors.Wrap(err, "unable to delete vault")
	}

	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(vault); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		fmt.Fprintf(s.Stdout, "Deleted vault %q.\n", vault.Name)
	}

	return &vault, nil
}
