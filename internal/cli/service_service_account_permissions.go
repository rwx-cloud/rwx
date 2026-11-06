package cli

import (
	"encoding/json"
	"fmt"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type ServiceAccountPermissionsResult struct {
	ServiceAccountID string
	Permissions      []PermissionInfo
}

type ShowServiceAccountPermissionsConfig struct {
	ServiceAccountSelector
	Json bool
}

type ReplaceServiceAccountPermissionsConfig struct {
	ServiceAccountSelector
	File string
	Yes  bool
	Json bool
}

type ClearServiceAccountPermissionsConfig struct {
	ServiceAccountSelector
	Yes  bool
	Json bool
}

func (s Service) ShowServiceAccountPermissions(cfg ShowServiceAccountPermissionsConfig) (*ServiceAccountPermissionsResult, error) {
	account, err := s.ResolveServiceAccount(cfg.ServiceAccountSelector)
	if err != nil {
		return nil, err
	}
	response, err := s.APIClient.GetServiceAccountPermissions(account.ID)
	if err != nil {
		return nil, errors.Wrap(err, "unable to show service-account direct permissions")
	}
	result := &ServiceAccountPermissionsResult{ServiceAccountID: account.ID, Permissions: permissionInfos(response.Permissions)}
	return result, s.writeServiceAccountPermissions(result, cfg.Json)
}

func (s Service) ReplaceServiceAccountPermissions(cfg ReplaceServiceAccountPermissionsConfig) (*ServiceAccountPermissionsResult, error) {
	desired, err := s.readPermissionReplacement(cfg.File)
	if err != nil {
		return nil, err
	}
	return s.replaceServiceAccountPermissions(cfg.ServiceAccountSelector, desired, cfg.Yes, cfg.Json)
}

func (s Service) ClearServiceAccountPermissions(cfg ClearServiceAccountPermissionsConfig) (*ServiceAccountPermissionsResult, error) {
	return s.replaceServiceAccountPermissions(cfg.ServiceAccountSelector, []api.Permission{}, cfg.Yes, cfg.Json)
}

func (s Service) replaceServiceAccountPermissions(selector ServiceAccountSelector, desired []api.Permission, yes, useJSON bool) (*ServiceAccountPermissionsResult, error) {
	account, err := s.ResolveServiceAccount(selector)
	if err != nil {
		return nil, err
	}
	current, err := s.APIClient.GetServiceAccountPermissions(account.ID)
	if err != nil {
		return nil, errors.Wrap(err, "unable to show service-account direct permissions")
	}
	catalog, err := s.APIClient.GetPermissionCatalog(api.GetPermissionCatalogConfig{ServiceAccountID: account.ID})
	if err != nil {
		return nil, errors.Wrap(err, "unable to get permission catalog")
	}
	if err := validatePermissionReplacement(desired, current.Permissions, catalog); err != nil {
		return nil, err
	}
	prompt := fmt.Sprintf("Replace all direct permissions for service account %q (%s) with %d permissions? Inherited group permissions are unchanged.", account.Name, account.ID, len(desired))
	if err := s.confirmDestruction(prompt, yes); err != nil {
		return nil, err
	}
	response, err := s.APIClient.ReplaceServiceAccountPermissions(api.ReplaceServiceAccountPermissionsConfig{ServiceAccountID: account.ID, Permissions: desired})
	if err != nil {
		return nil, errors.Wrap(err, "unable to replace service-account direct permissions")
	}
	result := &ServiceAccountPermissionsResult{ServiceAccountID: account.ID, Permissions: permissionInfos(response.Permissions)}
	return result, s.writeServiceAccountPermissions(result, useJSON)
}

func (s Service) writeServiceAccountPermissions(result *ServiceAccountPermissionsResult, useJSON bool) error {
	if useJSON {
		return errors.Wrap(json.NewEncoder(s.Stdout).Encode(result), "unable to encode JSON output")
	}
	fmt.Fprintf(s.Stdout, "Service account: %s\n", result.ServiceAccountID)
	return s.writeDirectPermissions(result.Permissions)
}
