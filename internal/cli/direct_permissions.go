package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"text/tabwriter"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type PermissionInfo struct {
	Key        string
	ResourceID *string
}

func (s Service) readPermissionReplacement(path string) ([]api.Permission, error) {
	if path == "" {
		return nil, errors.Wrap(errors.ErrBadRequest, "--file is required")
	}
	reader := s.Stdin
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return nil, errors.Wrap(err, "unable to open permission replacement file")
		}
		defer file.Close()
		reader = file
	}
	var envelope struct {
		Permissions []*api.Permission `json:"permissions"`
	}
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return nil, errors.Wrap(errors.WrapSentinel(err, errors.ErrBadRequest), "invalid permission replacement file")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.Wrap(errors.ErrBadRequest, "permission replacement file must contain exactly one JSON document")
	}
	if envelope.Permissions == nil {
		return nil, errors.Wrap(errors.ErrBadRequest, "permissions must be an array; use [] to explicitly clear direct permissions")
	}
	permissions := make([]api.Permission, len(envelope.Permissions))
	for i, permission := range envelope.Permissions {
		if permission == nil {
			return nil, errors.Wrap(errors.ErrBadRequest, "permissions must not contain null entries")
		}
		permissions[i] = *permission
	}
	return permissions, nil
}

func canonicalPermission(permission api.Permission) string {
	if permission.ResourceID != nil {
		return fmt.Sprintf("%s(id: %s)", permission.Key, *permission.ResourceID)
	}
	return permission.Key
}

func validatePermissionReplacement(desired, current []api.Permission, catalog *api.PermissionCatalog) error {
	retained := make(map[string]bool, len(current))
	for _, permission := range current {
		retained[canonicalPermission(permission)] = true
	}
	wanted := make(map[string]bool, len(desired))
	for _, permission := range desired {
		canonical := canonicalPermission(permission)
		index := slices.IndexFunc(catalog.Permissions, func(entry api.CatalogPermission) bool { return entry.Key == permission.Key })
		if index < 0 {
			return errors.Wrapf(errors.ErrBadRequest, "unknown permission key %q", permission.Key)
		}
		entry := catalog.Permissions[index]
		if permission.ResourceID != nil && (!entry.SupportsResourceScope || *permission.ResourceID == "") {
			return errors.Wrapf(errors.ErrBadRequest, "permission %q does not support this resource scope", canonical)
		}
		if wanted[canonical] {
			return errors.Wrapf(errors.ErrBadRequest, "duplicate permission %q", canonical)
		}
		wanted[canonical] = true
		if retained[canonical] {
			continue
		}
		grantable := entry.Grantable.Unscoped
		if permission.ResourceID != nil {
			grantable = slices.Contains(entry.Grantable.ResourceIDs, *permission.ResourceID)
		}
		if !grantable {
			return errors.Wrapf(errors.ErrBadRequest, "permission %q cannot be granted by the caller", canonical)
		}
	}
	for _, permission := range catalog.UnremovablePermissions {
		if !wanted[permission] {
			return errors.Wrapf(errors.ErrBadRequest, "permission %q cannot be removed by the caller", permission)
		}
	}
	return nil
}

func permissionInfos(permissions []api.Permission) []PermissionInfo {
	result := make([]PermissionInfo, len(permissions))
	for i, permission := range permissions {
		result[i] = PermissionInfo{Key: permission.Key, ResourceID: permission.ResourceID}
	}
	return result
}

func (s Service) writeDirectPermissions(permissions []PermissionInfo) error {
	fmt.Fprintln(s.Stdout, "Direct permissions (inherited permissions unchanged):")
	if len(permissions) == 0 {
		_, err := fmt.Fprintln(s.Stdout, "No direct permissions.")
		return err
	}
	w := tabwriter.NewWriter(s.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "KEY\tRESOURCE ID")
	for _, permission := range permissions {
		resource := "(unscoped)"
		if permission.ResourceID != nil {
			resource = *permission.ResourceID
		}
		fmt.Fprintf(w, "%s\t%s\n", permission.Key, resource)
	}
	return w.Flush()
}
