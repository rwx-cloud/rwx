package mocks

import (
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

func (c *API) ListGroups(cfg api.ListGroupsConfig) (*api.ListGroupsResult, error) {
	if c.MockListGroups != nil {
		return c.MockListGroups(cfg)
	}
	return nil, errors.New("MockListGroups was not configured")
}

func (c *API) ShowGroup(id string) (*api.GroupResult, error) {
	if c.MockShowGroup != nil {
		return c.MockShowGroup(id)
	}
	return nil, errors.New("MockShowGroup was not configured")
}

func (c *API) CreateGroup(cfg api.CreateGroupConfig) (*api.GroupResult, error) {
	if c.MockCreateGroup != nil {
		return c.MockCreateGroup(cfg)
	}
	return nil, errors.New("MockCreateGroup was not configured")
}

func (c *API) UpdateGroup(id string, cfg api.UpdateGroupConfig) (*api.GroupResult, error) {
	if c.MockUpdateGroup != nil {
		return c.MockUpdateGroup(id, cfg)
	}
	return nil, errors.New("MockUpdateGroup was not configured")
}

func (c *API) DeleteGroup(id string) error {
	if c.MockDeleteGroup != nil {
		return c.MockDeleteGroup(id)
	}
	return errors.New("MockDeleteGroup was not configured")
}
