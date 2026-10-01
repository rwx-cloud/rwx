package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type VaultAccessGrantInfo struct {
	ID                      string
	Vault                   VaultIdentity
	AccessType              string
	RepositorySlug          string
	RepositoryBranchPattern string
}

type ListVaultAccessConfig struct {
	Vault string
	Json  bool
}

type ListVaultAccessResult struct {
	AccessGrants []VaultAccessGrantInfo
}

type AllowVaultAccessConfig struct {
	Vault                   string
	RepositorySlug          string
	RepositoryBranchPattern string
	AllowFor                string
	ExpiresAt               string
	Json                    bool
}

type AllowVaultAccessResult = VaultAccessGrantInfo

type RevokeVaultAccessConfig struct {
	Vault                   string
	RepositorySlug          string
	RepositoryBranchPattern string
	Json                    bool
	Yes                     bool
}

type RevokeVaultAccessResult = VaultAccessGrantInfo

func vaultRepositoryAccessGrant(permission api.VaultRepositoryPermission) VaultAccessGrantInfo {
	return VaultAccessGrantInfo{
		ID: permission.ID,
		Vault: VaultIdentity{
			ID:   permission.Vault.ID,
			Name: permission.Vault.Name,
		},
		AccessType:              "repository",
		RepositorySlug:          permission.RepositorySlug,
		RepositoryBranchPattern: permission.BranchPattern,
	}
}

func validateRepositoryAccess(repositorySlug, repositoryBranchPattern string) error {
	if repositorySlug == "" {
		return errors.New("--repository-slug is required")
	}
	if repositoryBranchPattern == "" {
		return errors.New("--repository-branch-pattern is required")
	}
	return nil
}

func (s Service) ListVaultAccess(cfg ListVaultAccessConfig) (*ListVaultAccessResult, error) {
	vaultID, err := s.resolveVaultID(cfg.Vault)
	if err != nil {
		return nil, err
	}
	apiResult, err := s.APIClient.ListVaultRepositoryPermissions(api.ListVaultRepositoryPermissionsConfig{VaultID: vaultID})
	if err != nil {
		return nil, errors.Wrap(err, "unable to list vault access")
	}

	grants := make([]VaultAccessGrantInfo, len(apiResult.RepositoryPermissions))
	for i, permission := range apiResult.RepositoryPermissions {
		grants[i] = vaultRepositoryAccessGrant(permission)
	}
	result := &ListVaultAccessResult{AccessGrants: grants}
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else if len(grants) == 0 {
		fmt.Fprintf(s.Stdout, "No access grants found for vault %q.\n", cfg.Vault)
	} else {
		w := tabwriter.NewWriter(s.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TYPE\tIDENTITY\tBRANCH PATTERN")
		for _, grant := range grants {
			fmt.Fprintf(w, "%s\t%s\t%s\n", grant.AccessType, grant.RepositorySlug, grant.RepositoryBranchPattern)
		}
		if err := w.Flush(); err != nil {
			return nil, errors.Wrap(err, "unable to write access list")
		}
	}
	return result, nil
}

func (s Service) AllowVaultAccess(cfg AllowVaultAccessConfig) (*AllowVaultAccessResult, error) {
	if err := validateRepositoryAccess(cfg.RepositorySlug, cfg.RepositoryBranchPattern); err != nil {
		return nil, err
	}
	if cfg.AllowFor != "" || cfg.ExpiresAt != "" {
		fmt.Fprintln(s.Stderr, "Warning: repository access does not support expiration; --allow-for and --expires-at will be ignored.")
	}
	vaultID, err := s.resolveVaultID(cfg.Vault)
	if err != nil {
		return nil, err
	}
	apiResult, err := s.APIClient.CreateVaultRepositoryPermission(api.CreateVaultRepositoryPermissionConfig{
		VaultID:        vaultID,
		RepositorySlug: cfg.RepositorySlug,
		BranchPattern:  cfg.RepositoryBranchPattern,
	})
	if err != nil {
		return nil, errors.Wrap(err, "unable to allow repository access")
	}
	result := vaultRepositoryAccessGrant(*apiResult)
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		fmt.Fprintf(s.Stdout, "Allowed runs from repository %q matching %q to access vault %q.\n", result.RepositorySlug, result.RepositoryBranchPattern, result.Vault.Name)
	}
	return &result, nil
}

func (s Service) RevokeVaultAccess(cfg RevokeVaultAccessConfig) (*RevokeVaultAccessResult, error) {
	if err := validateRepositoryAccess(cfg.RepositorySlug, cfg.RepositoryBranchPattern); err != nil {
		return nil, err
	}
	vaultID, err := s.resolveVaultID(cfg.Vault)
	if err != nil {
		return nil, err
	}
	apiResult, err := s.APIClient.ListVaultRepositoryPermissions(api.ListVaultRepositoryPermissionsConfig{VaultID: vaultID})
	if err != nil {
		return nil, errors.Wrap(err, "unable to find repository access")
	}

	matches := make([]api.VaultRepositoryPermission, 0, 1)
	for _, permission := range apiResult.RepositoryPermissions {
		if permission.RepositorySlug == cfg.RepositorySlug && permission.BranchPattern == cfg.RepositoryBranchPattern {
			matches = append(matches, permission)
		}
	}
	if len(matches) == 0 {
		return nil, errors.Errorf("repository access for %q with branch pattern %q was not found", cfg.RepositorySlug, cfg.RepositoryBranchPattern)
	}
	if len(matches) > 1 {
		return nil, errors.Errorf("more than one repository access grant matches %q with branch pattern %q", cfg.RepositorySlug, cfg.RepositoryBranchPattern)
	}

	result := vaultRepositoryAccessGrant(matches[0])
	if err := s.confirmDestruction(fmt.Sprintf("Revoke repository access for %q with branch pattern %q from vault %q?", result.RepositorySlug, result.RepositoryBranchPattern, result.Vault.Name), cfg.Yes); err != nil {
		return nil, err
	}
	if _, err := s.APIClient.DeleteVaultRepositoryPermission(api.DeleteVaultRepositoryPermissionConfig{
		VaultID:      vaultID,
		PermissionID: result.ID,
	}); err != nil {
		return nil, errors.Wrap(err, "unable to revoke repository access")
	}
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		fmt.Fprintf(s.Stdout, "Revoked repository access for %q with branch pattern %q from vault %q.\n", result.RepositorySlug, result.RepositoryBranchPattern, result.Vault.Name)
	}
	return &result, nil
}
