package mocks

import (
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

func (c *API) ShowGroupRelationship(pair api.GroupRelationship) (*api.GroupRelationshipResult, error) {
	if c.MockShowGroupRelationship != nil {
		return c.MockShowGroupRelationship(pair)
	}
	return nil, errors.New("MockShowGroupRelationship was not configured")
}

func (c *API) AddGroupRelationship(pair api.GroupRelationship) (*api.GroupRelationshipResult, error) {
	if c.MockAddGroupRelationship != nil {
		return c.MockAddGroupRelationship(pair)
	}
	return nil, errors.New("MockAddGroupRelationship was not configured")
}

func (c *API) RemoveGroupRelationship(pair api.GroupRelationship) error {
	if c.MockRemoveGroupRelationship != nil {
		return c.MockRemoveGroupRelationship(pair)
	}
	return errors.New("MockRemoveGroupRelationship was not configured")
}
