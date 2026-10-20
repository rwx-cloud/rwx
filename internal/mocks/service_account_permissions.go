package mocks

import (
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

func (c *API) GetServiceAccountPermissions(id string) (*api.ServiceAccountPermissionsResult, error) {
	if c.MockGetServiceAccountPermissions != nil {
		return c.MockGetServiceAccountPermissions(id)
	}
	return nil, errors.New("MockGetServiceAccountPermissions was not configured")
}

func (c *API) ReplaceServiceAccountPermissions(cfg api.ReplaceServiceAccountPermissionsConfig) (*api.ServiceAccountPermissionsResult, error) {
	if c.MockReplaceServiceAccountPermissions != nil {
		return c.MockReplaceServiceAccountPermissions(cfg)
	}
	return nil, errors.New("MockReplaceServiceAccountPermissions was not configured")
}
