package mocks

import (
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

func (c *API) BillingUsage(month string) (*api.BillingUsageResult, error) {
	if c.MockBillingUsage != nil {
		return c.MockBillingUsage(month)
	}
	return nil, errors.New("MockBillingUsage was not configured")
}
