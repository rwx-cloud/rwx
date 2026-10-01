package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type VaultAccessGrantInfo struct {
	ID                      string
	Vault                   VaultIdentity
	AccessType              string
	RepositorySlug          string   `json:",omitempty"`
	RepositoryBranchPattern string   `json:",omitempty"`
	UserID                  string   `json:",omitempty"`
	Email                   string   `json:",omitempty"`
	ServiceAccountID        string   `json:",omitempty"`
	ServiceAccountName      string   `json:",omitempty"`
	ExpiresAt               **string `json:",omitempty"`
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
	Email                   string
	ServiceAccount          string
	AllowFor                string
	ExpiresAt               string
	Json                    bool
}

type AllowVaultAccessResult = VaultAccessGrantInfo

type RevokeVaultAccessConfig struct {
	Vault                   string
	RepositorySlug          string
	RepositoryBranchPattern string
	Email                   string
	ServiceAccount          string
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

func vaultPrincipalAccessGrant(grant api.VaultAccessGrant) VaultAccessGrantInfo {
	result := VaultAccessGrantInfo{
		ID:         grant.ID,
		Vault:      VaultIdentity{ID: grant.Vault.ID, Name: grant.Vault.Name},
		AccessType: grant.Principal.Type,
		ExpiresAt:  &grant.ExpiresAt,
	}
	if grant.Principal.Type == "user" {
		result.UserID = grant.Principal.ID
		result.Email = grant.Principal.Email
	} else {
		result.ServiceAccountID = grant.Principal.ID
		result.ServiceAccountName = grant.Principal.Name
	}
	return result
}

func validateVaultAccessSubject(repositorySlug, repositoryBranchPattern, email, serviceAccount string) (string, error) {
	repository := repositorySlug != "" || repositoryBranchPattern != ""
	subjects := 0
	for _, selected := range []bool{repository, email != "", serviceAccount != ""} {
		if selected {
			subjects++
		}
	}
	if subjects != 1 {
		return "", errors.New("exactly one of --repository-slug, --email, or --service-account is required")
	}
	if repository {
		if repositorySlug == "" {
			return "", errors.New("--repository-slug is required with --repository-branch-pattern")
		}
		if repositoryBranchPattern == "" {
			return "", errors.New("--repository-branch-pattern is required with --repository-slug")
		}
		return "repository", nil
	}
	if email != "" {
		return "user", nil
	}
	return "service_account", nil
}

func (s Service) ListVaultAccess(cfg ListVaultAccessConfig) (*ListVaultAccessResult, error) {
	vaultID, err := s.resolveVaultID(cfg.Vault)
	if err != nil {
		return nil, err
	}
	principalResult, err := s.APIClient.ListVaultAccessGrants(api.ListVaultAccessGrantsConfig{VaultID: vaultID})
	if err != nil {
		return nil, errors.Wrap(err, "unable to list vault access")
	}
	repositoryResult, err := s.APIClient.ListVaultRepositoryPermissions(api.ListVaultRepositoryPermissionsConfig{VaultID: vaultID})
	if err != nil {
		return nil, errors.Wrap(err, "unable to list vault access")
	}

	grants := make([]VaultAccessGrantInfo, 0, len(principalResult.AccessGrants)+len(repositoryResult.RepositoryPermissions))
	for _, grant := range principalResult.AccessGrants {
		grants = append(grants, vaultPrincipalAccessGrant(grant))
	}
	for _, permission := range repositoryResult.RepositoryPermissions {
		grants = append(grants, vaultRepositoryAccessGrant(permission))
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
		fmt.Fprintln(w, "TYPE\tIDENTITY\tBRANCH PATTERN\tEXPIRES AT")
		for _, grant := range grants {
			identity := grant.RepositorySlug
			expiresAt := ""
			if grant.AccessType == "user" {
				identity = grant.Email
				expiresAt = "never"
			} else if grant.AccessType == "service_account" {
				identity = grant.ServiceAccountName
				expiresAt = "never"
			}
			if grant.ExpiresAt != nil && *grant.ExpiresAt != nil {
				expiresAt = **grant.ExpiresAt
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", grant.AccessType, identity, grant.RepositoryBranchPattern, expiresAt)
		}
		if err := w.Flush(); err != nil {
			return nil, errors.Wrap(err, "unable to write access list")
		}
	}
	return result, nil
}

func (s Service) AllowVaultAccess(cfg AllowVaultAccessConfig) (*AllowVaultAccessResult, error) {
	accessType, err := validateVaultAccessSubject(cfg.RepositorySlug, cfg.RepositoryBranchPattern, cfg.Email, cfg.ServiceAccount)
	if err != nil {
		return nil, err
	}
	if cfg.AllowFor != "" && cfg.ExpiresAt != "" {
		return nil, errors.New("--allow-for and --expires-at are mutually exclusive")
	}
	if accessType == "repository" && (cfg.AllowFor != "" || cfg.ExpiresAt != "") {
		fmt.Fprintln(s.Stderr, "Warning: repository access does not support expiration; --allow-for and --expires-at will be ignored.")
	}
	vaultID, err := s.resolveVaultID(cfg.Vault)
	if err != nil {
		return nil, err
	}

	var result VaultAccessGrantInfo
	if accessType == "repository" {
		apiResult, err := s.APIClient.CreateVaultRepositoryPermission(api.CreateVaultRepositoryPermissionConfig{
			VaultID: vaultID, RepositorySlug: cfg.RepositorySlug, BranchPattern: cfg.RepositoryBranchPattern,
		})
		if err != nil {
			return nil, errors.Wrap(err, "unable to allow repository access")
		}
		result = vaultRepositoryAccessGrant(*apiResult)
	} else {
		var expiresAt *string
		if cfg.AllowFor != "" {
			duration, err := time.ParseDuration(cfg.AllowFor)
			if err != nil {
				return nil, errors.Wrap(err, "--allow-for must be a duration")
			}
			if duration <= 0 {
				return nil, errors.New("--allow-for must be greater than zero")
			}
			value := time.Now().Add(duration).UTC().Format(time.RFC3339Nano)
			expiresAt = &value
		} else if cfg.ExpiresAt != "" {
			expiresAt = &cfg.ExpiresAt
		}
		apiResult, err := s.APIClient.CreateVaultAccessGrant(api.CreateVaultAccessGrantConfig{
			VaultID: vaultID, PrincipalType: accessType, Email: cfg.Email, ServiceAccount: cfg.ServiceAccount, ExpiresAt: expiresAt,
		})
		if err != nil {
			return nil, errors.Wrap(err, "unable to allow vault access")
		}
		result = vaultPrincipalAccessGrant(apiResult.AccessGrant)
	}
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else if accessType == "repository" {
		fmt.Fprintf(s.Stdout, "Allowed runs from repository %q matching %q to access vault %q.\n", result.RepositorySlug, result.RepositoryBranchPattern, result.Vault.Name)
	} else if accessType == "user" {
		fmt.Fprintf(s.Stdout, "Allowed user %q to access vault %q.\n", result.Email, result.Vault.Name)
	} else {
		fmt.Fprintf(s.Stdout, "Allowed service account %q to access vault %q.\n", result.ServiceAccountName, result.Vault.Name)
	}
	return &result, nil
}

func (s Service) RevokeVaultAccess(cfg RevokeVaultAccessConfig) (*RevokeVaultAccessResult, error) {
	accessType, err := validateVaultAccessSubject(cfg.RepositorySlug, cfg.RepositoryBranchPattern, cfg.Email, cfg.ServiceAccount)
	if err != nil {
		return nil, err
	}
	vaultID, err := s.resolveVaultID(cfg.Vault)
	if err != nil {
		return nil, err
	}
	if accessType != "repository" {
		apiResult, err := s.APIClient.ListVaultAccessGrants(api.ListVaultAccessGrantsConfig{VaultID: vaultID})
		if err != nil {
			return nil, errors.Wrap(err, "unable to find vault access")
		}
		var match *api.VaultAccessGrant
		for i := range apiResult.AccessGrants {
			grant := &apiResult.AccessGrants[i]
			if (accessType == "user" && grant.Principal.Type == "user" && grant.Principal.Email == cfg.Email) ||
				(accessType == "service_account" && grant.Principal.Type == "service_account" && grant.Principal.Name == cfg.ServiceAccount) {
				match = grant
				break
			}
		}
		if match == nil {
			if accessType == "user" {
				return nil, errors.Errorf("user access for %q was not found", cfg.Email)
			}
			return nil, errors.Errorf("service account access for %q was not found", cfg.ServiceAccount)
		}
		result := vaultPrincipalAccessGrant(*match)
		identity := result.Email
		accessLabel := accessType
		if accessType == "service_account" {
			identity = result.ServiceAccountName
			accessLabel = "service account"
		}
		if err := s.confirmDestruction(fmt.Sprintf("Revoke %s access for %q from vault %q?", accessLabel, identity, result.Vault.Name), cfg.Yes); err != nil {
			return nil, err
		}
		if _, err := s.APIClient.DeleteVaultAccessGrant(api.DeleteVaultAccessGrantConfig{VaultID: vaultID, GrantID: result.ID}); err != nil {
			return nil, errors.Wrap(err, "unable to revoke vault access")
		}
		if cfg.Json {
			if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
				return nil, errors.Wrap(err, "unable to encode JSON output")
			}
		} else if accessType == "user" {
			fmt.Fprintf(s.Stdout, "Revoked user access for %q from vault %q.\n", result.Email, result.Vault.Name)
		} else {
			fmt.Fprintf(s.Stdout, "Revoked service account access for %q from vault %q.\n", result.ServiceAccountName, result.Vault.Name)
		}
		return &result, nil
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
