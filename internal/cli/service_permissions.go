package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type ListPermissionsConfig struct {
	GroupID          string
	ServiceAccountID string
	Json             bool
}

type PermissionGrantabilityInfo struct {
	Unscoped    bool
	ResourceIDs []string
}

type CatalogPermissionInfo struct {
	Key                   string
	SupportsResourceScope bool
	ScopeResource         *string
	Grantable             PermissionGrantabilityInfo
}

type ListPermissionsResult struct {
	Permissions            []CatalogPermissionInfo
	UnremovablePermissions []string
}

func (s Service) ListPermissions(cfg ListPermissionsConfig) (*ListPermissionsResult, error) {
	if cfg.GroupID != "" && cfg.ServiceAccountID != "" {
		return nil, errors.Wrap(errors.ErrBadRequest, "--group-id and --service-account-id cannot be used together")
	}
	catalog, err := s.APIClient.GetPermissionCatalog(api.GetPermissionCatalogConfig{
		GroupID: cfg.GroupID, ServiceAccountID: cfg.ServiceAccountID,
	})
	if err != nil {
		return nil, errors.Wrap(err, "unable to list permissions")
	}
	result := &ListPermissionsResult{
		Permissions:            make([]CatalogPermissionInfo, len(catalog.Permissions)),
		UnremovablePermissions: append([]string{}, catalog.UnremovablePermissions...),
	}
	for i, permission := range catalog.Permissions {
		result.Permissions[i] = CatalogPermissionInfo{
			Key:                   permission.Key,
			SupportsResourceScope: permission.SupportsResourceScope,
			ScopeResource:         permission.ScopeResource,
			Grantable: PermissionGrantabilityInfo{
				Unscoped:    permission.Grantable.Unscoped,
				ResourceIDs: append([]string{}, permission.Grantable.ResourceIDs...),
			},
		}
	}
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		if len(result.Permissions) == 0 {
			fmt.Fprintln(s.Stdout, "No permissions found.")
		} else {
			writer := tabwriter.NewWriter(s.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(writer, "KEY\tRESOURCE SCOPE\tSCOPE RESOURCE\tCALLER CAN GRANT UNSCOPED\tCALLER CAN GRANT RESOURCE IDS")
			for _, permission := range result.Permissions {
				resource := "-"
				if permission.ScopeResource != nil {
					resource = *permission.ScopeResource
				}
				ids := strings.Join(permission.Grantable.ResourceIDs, ", ")
				if ids == "" {
					ids = "-"
				}
				fmt.Fprintf(writer, "%s\t%t\t%s\t%t\t%s\n", permission.Key, permission.SupportsResourceScope, resource, permission.Grantable.Unscoped, ids)
			}
			if err := writer.Flush(); err != nil {
				return nil, errors.Wrap(err, "unable to write permission catalog")
			}
		}
		if len(result.UnremovablePermissions) > 0 {
			fmt.Fprintln(s.Stdout, "\nTarget permissions the caller cannot remove:")
			for _, permission := range result.UnremovablePermissions {
				fmt.Fprintln(s.Stdout, permission)
			}
		}
	}
	return result, nil
}
