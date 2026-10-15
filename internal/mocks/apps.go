package mocks

import (
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

func (c *API) ShareApp(cfg api.ShareAppConfig) (*api.ShareAppResult, error) {
	if c.MockShareApp != nil {
		return c.MockShareApp(cfg)
	}
	return nil, errors.New("MockShareApp was not configured")
}

func (c *API) DisableApp(endpoint string) error {
	if c.MockDisableApp != nil {
		return c.MockDisableApp(endpoint)
	}
	return errors.New("MockDisableApp was not configured")
}

func (c *API) EnableApp(endpoint string) error {
	if c.MockEnableApp != nil {
		return c.MockEnableApp(endpoint)
	}
	return errors.New("MockEnableApp was not configured")
}
