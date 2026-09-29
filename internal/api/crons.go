package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/rwx-cloud/rwx/internal/errors"
)

type Cron struct {
	ID                string  `json:"id"`
	Key               string  `json:"key"`
	Repository        string  `json:"repository"`
	Branch            string  `json:"branch"`
	RunDefinitionPath string  `json:"run_definition_path"`
	Schedule          string  `json:"schedule"`
	TimeZone          string  `json:"time_zone"`
	Status            string  `json:"status"`
	NextInvocationAt  *string `json:"next_invocation_at"`
	SnoozedUntil      *string `json:"snoozed_until"`
}

type ListCronsResult struct {
	Crons []Cron `json:"crons"`
}
type CronResult struct {
	Cron Cron `json:"cron"`
}

func (c Client) ListCrons() (*ListCronsResult, error) {
	result := &ListCronsResult{}
	if err := c.cronRequest(http.MethodGet, "/mint/api/crons", nil, http.StatusOK, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c Client) ShowCron(id string) (*CronResult, error) {
	return c.cronDetails(http.MethodGet, id, "", nil)
}

func (c Client) PauseCron(id string) (*CronResult, error) {
	return c.cronDetails(http.MethodPost, id, "/pause", nil)
}

func (c Client) ResumeCron(id string) (*CronResult, error) {
	return c.cronDetails(http.MethodPost, id, "/resume", nil)
}

func (c Client) SnoozeCron(id, until string) (*CronResult, error) {
	return c.cronDetails(http.MethodPost, id, "/snooze", struct {
		Until string `json:"until"`
	}{Until: until})
}

func (c Client) cronDetails(method, id, action string, body any) (*CronResult, error) {
	result := &CronResult{}
	if err := c.cronRequest(method, "/mint/api/crons/"+url.PathEscape(id)+action, body, http.StatusOK, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c Client) cronRequest(method, endpoint string, body any, status int, result any) error {
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
	if resp.StatusCode != status {
		msg := extractErrorMessage(resp.Body)
		if msg == "" {
			msg = fmt.Sprintf("Unable to call RWX API - %s", resp.Status)
		}
		return classifyHTTPStatusError(resp.StatusCode, msg)
	}
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return errors.Wrap(err, "unable to parse API response")
	}
	return nil
}
