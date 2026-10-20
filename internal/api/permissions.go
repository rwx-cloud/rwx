package api

import (
	"net/http"
	"net/url"

	"github.com/rwx-cloud/rwx/internal/errors"
)

type Permission struct {
	Key        string  `json:"key"`
	ResourceID *string `json:"resource_id,omitempty"`
}

type PermissionGrantability struct {
	Unscoped    bool     `json:"unscoped"`
	ResourceIDs []string `json:"resource_ids"`
}

type CatalogPermission struct {
	Key                   string                 `json:"key"`
	SupportsResourceScope bool                   `json:"supports_resource_scope"`
	ScopeResource         *string                `json:"scope_resource"`
	Grantable             PermissionGrantability `json:"grantable"`
}

type PermissionCatalog struct {
	Permissions            []CatalogPermission `json:"permissions"`
	UnremovablePermissions []string            `json:"unremovable_permissions"`
}

type GetPermissionCatalogConfig struct {
	GroupID          string
	ServiceAccountID string
}

func (c Client) GetPermissionCatalog(cfg GetPermissionCatalogConfig) (*PermissionCatalog, error) {
	if cfg.GroupID != "" && cfg.ServiceAccountID != "" {
		return nil, errors.New("provide group_id or service_account_id, not both")
	}
	query := url.Values{}
	if cfg.GroupID != "" {
		query.Set("group_id", cfg.GroupID)
	}
	if cfg.ServiceAccountID != "" {
		query.Set("service_account_id", cfg.ServiceAccountID)
	}
	endpoint := "/mint/api/permission_catalog"
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create new HTTP request")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.RoundTrip(req)
	if err != nil {
		return nil, errors.Wrap(err, "HTTP request failed")
	}
	defer resp.Body.Close()
	result := &PermissionCatalog{}
	if err := decodeResponseJSON(resp, result); err != nil {
		return nil, err
	}
	return result, nil
}
