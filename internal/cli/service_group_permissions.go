package cli

import (
	"encoding/json"
	"fmt"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type ShowGroupPermissionsConfig struct {
	Group string
	Json  bool
}

type ReplaceGroupPermissionsConfig struct {
	Group string
	File  string
	Yes   bool
	Json  bool
}

type ClearGroupPermissionsConfig struct {
	Group string
	Yes   bool
	Json  bool
}

type GroupPermissionsResult struct {
	GroupID     string
	Permissions []PermissionInfo
}

func (s Service) ShowGroupPermissions(cfg ShowGroupPermissionsConfig) (*GroupPermissionsResult, error) {
	group, err := s.ResolveGroup(cfg.Group)
	if err != nil {
		return nil, err
	}
	result, err := s.APIClient.GetGroupPermissions(group.ID)
	if err != nil {
		return nil, errors.Wrap(err, "unable to read group direct permissions")
	}
	return s.outputGroupPermissions(group.ID, result.Permissions, cfg.Json)
}

func (s Service) ReplaceGroupPermissions(cfg ReplaceGroupPermissionsConfig) (*GroupPermissionsResult, error) {
	permissions, err := s.readPermissionReplacement(cfg.File)
	if err != nil {
		return nil, err
	}
	return s.replaceGroupPermissions(cfg.Group, permissions, cfg.Yes, cfg.Json)
}

func (s Service) ClearGroupPermissions(cfg ClearGroupPermissionsConfig) (*GroupPermissionsResult, error) {
	return s.replaceGroupPermissions(cfg.Group, []api.Permission{}, cfg.Yes, cfg.Json)
}

func (s Service) replaceGroupPermissions(selector string, permissions []api.Permission, yes, jsonOutput bool) (*GroupPermissionsResult, error) {
	group, err := s.ResolveGroup(selector)
	if err != nil {
		return nil, err
	}
	current, err := s.APIClient.GetGroupPermissions(group.ID)
	if err != nil {
		return nil, errors.Wrap(err, "unable to read group direct permissions")
	}
	catalog, err := s.APIClient.GetPermissionCatalog(api.GetPermissionCatalogConfig{GroupID: group.ID})
	if err != nil {
		return nil, errors.Wrap(err, "unable to read permission catalog")
	}
	if err := validatePermissionReplacement(permissions, current.Permissions, catalog); err != nil {
		return nil, err
	}
	prompt := fmt.Sprintf("Replace the complete direct-permission set for group %q (%s) with %d permissions? Inherited permissions remain unchanged.", group.Name, group.ID, len(permissions))
	if err := s.confirmDestruction(prompt, yes); err != nil {
		return nil, err
	}
	result, err := s.APIClient.ReplaceGroupPermissions(group.ID, permissions)
	if err != nil {
		return nil, errors.Wrap(err, "unable to replace group direct permissions")
	}
	return s.outputGroupPermissions(group.ID, result.Permissions, jsonOutput)
}

func (s Service) outputGroupPermissions(id string, permissions []api.Permission, jsonOutput bool) (*GroupPermissionsResult, error) {
	result := &GroupPermissionsResult{GroupID: id, Permissions: permissionInfos(permissions)}
	if jsonOutput {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, err
		}
	} else {
		fmt.Fprintf(s.Stdout, "Group: %s\n", id)
		if err := s.writeDirectPermissions(result.Permissions); err != nil {
			return nil, err
		}
	}
	return result, nil
}
