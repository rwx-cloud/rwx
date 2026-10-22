package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/rwx-cloud/rwx/internal/errors"
)

type Group struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type GroupPagination struct {
	NextCursor *string `json:"next_cursor"`
	Limit      int     `json:"limit"`
}

type ListGroupsConfig struct {
	Limit  int
	Cursor string
}

type ListGroupsResult struct {
	Groups     []Group         `json:"groups"`
	Pagination GroupPagination `json:"pagination"`
}

type GroupResult struct {
	Group Group `json:"group"`
}

type CreateGroupConfig struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

type UpdateGroupConfig struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

func (c Client) ListGroups(cfg ListGroupsConfig) (*ListGroupsResult, error) {
	params := url.Values{}
	if cfg.Limit > 0 {
		params.Set("limit", strconv.Itoa(cfg.Limit))
	}
	if cfg.Cursor != "" {
		params.Set("cursor", cfg.Cursor)
	}
	result := &ListGroupsResult{}
	if err := c.groupRequest(http.MethodGet, "/mint/api/groups?"+params.Encode(), nil, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c Client) ShowGroup(id string) (*GroupResult, error) {
	result := &GroupResult{}
	if err := c.groupRequest(http.MethodGet, "/mint/api/groups/"+url.PathEscape(id), nil, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c Client) CreateGroup(cfg CreateGroupConfig) (*GroupResult, error) {
	result := &GroupResult{}
	if err := c.groupRequest(http.MethodPost, "/mint/api/groups", cfg, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c Client) UpdateGroup(id string, cfg UpdateGroupConfig) (*GroupResult, error) {
	result := &GroupResult{}
	if err := c.groupRequest(http.MethodPatch, "/mint/api/groups/"+url.PathEscape(id), cfg, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c Client) DeleteGroup(id string) error {
	return c.groupRequest(http.MethodDelete, "/mint/api/groups/"+url.PathEscape(id), nil, nil)
}

func (c Client) groupRequest(method, endpoint string, body, result any) error {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return errors.Wrap(err, "unable to encode group request")
		}
	}
	req, err := http.NewRequest(method, endpoint, bytes.NewReader(data))
	if err != nil {
		return errors.Wrap(err, "unable to create new HTTP request")
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.RoundTrip(req)
	if err != nil {
		return errors.Wrap(err, "HTTP request failed")
	}
	defer resp.Body.Close()
	return decodeResponseJSON(resp, result)
}
