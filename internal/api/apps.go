package api

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/rwx-cloud/rwx/internal/errors"
)

type ShareAppConfig struct {
	Endpoint    string `json:"endpoint"`
	Description string `json:"description,omitempty"`
	ExpiresAt   string `json:"expires_at"`
}

type ShareAppResult struct {
	URL         string `json:"share_url"`
	ExpiresAt   string `json:"expires_at"`
	Description string `json:"description"`
}

func (c Client) ShareApp(cfg ShareAppConfig) (*ShareAppResult, error) {
	var encoded bytes.Buffer
	if err := json.NewEncoder(&encoded).Encode(cfg); err != nil {
		return nil, errors.Wrap(err, "unable to encode as JSON")
	}

	req, err := http.NewRequest(http.MethodPost, "/mint/api/preview_app_shares", &encoded)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create new HTTP request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.RoundTrip(req)
	if err != nil {
		return nil, errors.Wrap(err, "HTTP request failed")
	}
	defer resp.Body.Close()
	result := &ShareAppResult{}
	if err := decodeResponseJSON(resp, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c Client) DisableApp(endpoint string) error {
	return c.appRequest(endpoint, "disable")
}

func (c Client) EnableApp(endpoint string) error {
	return c.appRequest(endpoint, "enable")
}

func (c Client) appRequest(endpoint, action string) error {
	var encoded bytes.Buffer
	if err := json.NewEncoder(&encoded).Encode(struct {
		Endpoint string `json:"endpoint"`
	}{Endpoint: endpoint}); err != nil {
		return errors.Wrap(err, "unable to encode as JSON")
	}

	req, err := http.NewRequest(http.MethodPost, "/mint/api/app_endpoints/"+action, &encoded)
	if err != nil {
		return errors.Wrap(err, "unable to create new HTTP request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.RoundTrip(req)
	if err != nil {
		return errors.Wrap(err, "HTTP request failed")
	}
	defer resp.Body.Close()
	return decodeResponseJSON(resp, nil)
}
