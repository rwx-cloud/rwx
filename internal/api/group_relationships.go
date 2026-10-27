package api

import (
	"net/http"
	"net/url"
)

type GroupRelationship struct {
	ParentGroupID string `json:"parent_group_id"`
	ChildGroupID  string `json:"child_group_id"`
}

type GroupRelationshipResult struct {
	GroupRelationship GroupRelationship `json:"group_relationship"`
}

func (c Client) ShowGroupRelationship(pair GroupRelationship) (*GroupRelationshipResult, error) {
	params := url.Values{"parent_group_id": {pair.ParentGroupID}, "child_group_id": {pair.ChildGroupID}}
	result := &GroupRelationshipResult{}
	if err := c.groupRequest(http.MethodGet, "/mint/api/group_relationships?"+params.Encode(), nil, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c Client) AddGroupRelationship(pair GroupRelationship) (*GroupRelationshipResult, error) {
	result := &GroupRelationshipResult{}
	if err := c.groupRequest(http.MethodPost, "/mint/api/group_relationships", pair, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c Client) RemoveGroupRelationship(pair GroupRelationship) error {
	return c.groupRequest(http.MethodDelete, "/mint/api/group_relationships", pair, nil)
}
