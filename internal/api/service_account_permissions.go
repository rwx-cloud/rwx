package api

import (
	"net/http"
	"net/url"
)

type ServiceAccountPermissionsResult struct {
	Permissions []Permission `json:"permissions"`
}

type ReplaceServiceAccountPermissionsConfig struct {
	ServiceAccountID string       `json:"-"`
	Permissions      []Permission `json:"permissions"`
}

func (c Client) GetServiceAccountPermissions(id string) (*ServiceAccountPermissionsResult, error) {
	result := &ServiceAccountPermissionsResult{Permissions: []Permission{}}
	if err := c.serviceAccountRequest(http.MethodGet, "/mint/api/service_accounts/"+url.PathEscape(id)+"/permissions", nil, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c Client) ReplaceServiceAccountPermissions(cfg ReplaceServiceAccountPermissionsConfig) (*ServiceAccountPermissionsResult, error) {
	if cfg.Permissions == nil {
		cfg.Permissions = []Permission{}
	}
	result := &ServiceAccountPermissionsResult{Permissions: []Permission{}}
	if err := c.serviceAccountRequest(http.MethodPut, "/mint/api/service_accounts/"+url.PathEscape(cfg.ServiceAccountID)+"/permissions", cfg, result); err != nil {
		return nil, err
	}
	return result, nil
}
