package api

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/rwx-cloud/rwx/internal/errors"
)

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
