package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/rwx-cloud/rwx/internal/errors"
)

type ServiceAccount struct {
	ID          string  `json:"id"`
	IdentityID  string  `json:"identity_id"`
	Name        string  `json:"name"`
	DisplayName string  `json:"display_name"`
	Description *string `json:"description"`
}

type ListServiceAccountsConfig struct {
	Name        string
	DisplayName string
	Limit       int
	Cursor      string
}

type ListServiceAccountsResult struct {
	ServiceAccounts []ServiceAccount `json:"service_accounts"`
	Pagination      struct {
		NextCursor string `json:"next_cursor"`
		Limit      int    `json:"limit"`
	} `json:"pagination"`
}

type ServiceAccountResult struct {
	ServiceAccount ServiceAccount `json:"service_account"`
}

type CreateServiceAccountConfig struct {
	DisplayName string  `json:"display_name"`
	Description *string `json:"description,omitempty"`
}

type UpdateServiceAccountConfig struct {
	ID          string  `json:"-"`
	DisplayName *string `json:"display_name,omitempty"`
	Description *string `json:"description,omitempty"`
}

func (c Client) ListServiceAccounts(cfg ListServiceAccountsConfig) (*ListServiceAccountsResult, error) {
	query := url.Values{}
	if cfg.Name != "" {
		query.Set("name", cfg.Name)
	}
	if cfg.DisplayName != "" {
		query.Set("display_name", cfg.DisplayName)
	}
	if cfg.Limit != 0 {
		query.Set("limit", strconv.Itoa(cfg.Limit))
	}
	if cfg.Cursor != "" {
		query.Set("cursor", cfg.Cursor)
	}
	result := &ListServiceAccountsResult{ServiceAccounts: []ServiceAccount{}}
	if err := c.serviceAccountRequest(http.MethodGet, "/mint/api/service_accounts?"+query.Encode(), nil, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c Client) ShowServiceAccount(id string) (*ServiceAccountResult, error) {
	result := &ServiceAccountResult{}
	if err := c.serviceAccountRequest(http.MethodGet, "/mint/api/service_accounts/"+url.PathEscape(id), nil, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c Client) CreateServiceAccount(cfg CreateServiceAccountConfig) (*ServiceAccountResult, error) {
	result := &ServiceAccountResult{}
	if err := c.serviceAccountRequest(http.MethodPost, "/mint/api/service_accounts", cfg, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c Client) UpdateServiceAccount(cfg UpdateServiceAccountConfig) (*ServiceAccountResult, error) {
	result := &ServiceAccountResult{}
	if err := c.serviceAccountRequest(http.MethodPatch, "/mint/api/service_accounts/"+url.PathEscape(cfg.ID), cfg, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c Client) DeactivateServiceAccount(id string) error {
	return c.serviceAccountRequest(http.MethodDelete, "/mint/api/service_accounts/"+url.PathEscape(id), nil, nil)
}

func (c Client) serviceAccountRequest(method, endpoint string, body, result any) error {
	var encoded bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&encoded).Encode(body); err != nil {
			return errors.Wrap(err, "unable to encode as JSON")
		}
	}
	req, err := http.NewRequest(method, endpoint, &encoded)
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
