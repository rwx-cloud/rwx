package api

import (
	"net/http"
	"net/url"
)

type GroupPermissionsResult struct {
	Permissions []Permission `json:"permissions"`
}

func (c Client) GetGroupPermissions(id string) (*GroupPermissionsResult, error) {
	result := &GroupPermissionsResult{}
	if err := c.groupRequest(http.MethodGet, "/mint/api/groups/"+url.PathEscape(id)+"/permissions", nil, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c Client) ReplaceGroupPermissions(id string, permissions []Permission) (*GroupPermissionsResult, error) {
	if permissions == nil {
		permissions = []Permission{}
	}
	result := &GroupPermissionsResult{}
	if err := c.groupRequest(http.MethodPut, "/mint/api/groups/"+url.PathEscape(id)+"/permissions", GroupPermissionsResult{Permissions: permissions}, result); err != nil {
		return nil, err
	}
	return result, nil
}
