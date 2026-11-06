package mocks

import (
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

func (c *API) ListServiceAccounts(cfg api.ListServiceAccountsConfig) (*api.ListServiceAccountsResult, error) {
	if c.MockListServiceAccounts != nil {
		return c.MockListServiceAccounts(cfg)
	}
	return nil, errors.New("MockListServiceAccounts was not configured")
}

func (c *API) ShowServiceAccount(id string) (*api.ServiceAccountResult, error) {
	if c.MockShowServiceAccount != nil {
		return c.MockShowServiceAccount(id)
	}
	return nil, errors.New("MockShowServiceAccount was not configured")
}

func (c *API) CreateServiceAccount(cfg api.CreateServiceAccountConfig) (*api.ServiceAccountResult, error) {
	if c.MockCreateServiceAccount != nil {
		return c.MockCreateServiceAccount(cfg)
	}
	return nil, errors.New("MockCreateServiceAccount was not configured")
}

func (c *API) UpdateServiceAccount(cfg api.UpdateServiceAccountConfig) (*api.ServiceAccountResult, error) {
	if c.MockUpdateServiceAccount != nil {
		return c.MockUpdateServiceAccount(cfg)
	}
	return nil, errors.New("MockUpdateServiceAccount was not configured")
}

func (c *API) DeactivateServiceAccount(id string) error {
	if c.MockDeactivateServiceAccount != nil {
		return c.MockDeactivateServiceAccount(id)
	}
	return errors.New("MockDeactivateServiceAccount was not configured")
}
