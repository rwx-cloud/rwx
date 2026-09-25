package mocks

import (
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

func (c *API) ListCrons() (*api.ListCronsResult, error) {
	if c.MockListCrons != nil {
		return c.MockListCrons()
	}
	return nil, errors.New("MockListCrons was not configured")
}
func (c *API) ShowCron(id string) (*api.CronResult, error) {
	if c.MockShowCron != nil {
		return c.MockShowCron(id)
	}
	return nil, errors.New("MockShowCron was not configured")
}
func (c *API) PauseCron(id string) (*api.CronResult, error) {
	if c.MockPauseCron != nil {
		return c.MockPauseCron(id)
	}
	return nil, errors.New("MockPauseCron was not configured")
}
func (c *API) ResumeCron(id string) (*api.CronResult, error) {
	if c.MockResumeCron != nil {
		return c.MockResumeCron(id)
	}
	return nil, errors.New("MockResumeCron was not configured")
}
func (c *API) SnoozeCron(id, until string) (*api.CronResult, error) {
	if c.MockSnoozeCron != nil {
		return c.MockSnoozeCron(id, until)
	}
	return nil, errors.New("MockSnoozeCron was not configured")
}
