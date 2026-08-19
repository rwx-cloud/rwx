package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/rwx-cloud/rwx/internal/errors"
)

type RetryTargetType string

const (
	RetryTargetInferred RetryTargetType = "inferred"
	RetryTargetRun      RetryTargetType = "run"
	RetryTargetTask     RetryTargetType = "task"
)

type RetryTarget struct {
	ID   string
	Type RetryTargetType
}

type RetryKind struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type RetryDebugOptions struct {
	Supported        bool     `json:"supported"`
	Placements       []string `json:"placements"`
	DefaultPlacement string   `json:"default_placement,omitempty"`
	DisabledReason   string   `json:"disabled_reason,omitempty"`
}

type RetryToolCache struct {
	Name             string   `json:"name"`
	ScopedTaskKeys   []string `json:"scoped_task_keys"`
	UsageDescription string   `json:"usage_description"`
}

type RetryOptions struct {
	Retryable         bool              `json:"retryable"`
	UnavailableReason string            `json:"unavailable_reason,omitempty"`
	Kinds             []RetryKind       `json:"kinds"`
	Debug             RetryDebugOptions `json:"debug"`
	ToolCaches        []RetryToolCache  `json:"tool_caches"`
}

type RequestRetryConfig struct {
	Target         RetryTarget
	Kind           string
	Debug          *bool
	DebugPlacement string
	ToolCacheNames []string
}

type RequestRetryResult struct {
	Status string `json:"status"`
	RunID  string `json:"run_id"`
	TaskID string `json:"task_id,omitempty"`
}

type RetryFieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type RetryRequestError struct {
	Message string            `json:"error"`
	Errors  []RetryFieldError `json:"errors"`
	Options RetryOptions      `json:"options"`
}

func (e *RetryRequestError) Error() string {
	var message strings.Builder
	message.WriteString(e.Message)
	for _, fieldError := range e.Errors {
		fmt.Fprintf(&message, "\n  %s: %s", fieldError.Field, fieldError.Message)
	}
	return message.String()
}

func (e *RetryRequestError) Unwrap() error {
	return errors.ErrBadRequest
}

func (c Client) GetRetryOptions(target RetryTarget) (RetryOptions, error) {
	params, err := target.queryParams()
	if err != nil {
		return RetryOptions{}, err
	}

	endpoint := "/mint/api/retries/options?" + params.Encode()
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return RetryOptions{}, errors.Wrap(err, "unable to create new HTTP request")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.RoundTrip(req)
	if err != nil {
		return RetryOptions{}, errors.Wrap(err, "HTTP request failed")
	}
	defer resp.Body.Close()

	result := RetryOptions{}
	if err := decodeRetryResponseJSON(resp, &result); err != nil {
		return RetryOptions{}, err
	}
	return result, nil
}

func (c Client) RequestRetry(cfg RequestRetryConfig) (RequestRetryResult, error) {
	params, err := cfg.Target.queryParams()
	if err != nil {
		return RequestRetryResult{}, err
	}
	if cfg.Kind == "" {
		return RequestRetryResult{}, badRetryRequest("missing retry kind")
	}

	requestBody := struct {
		Retry struct {
			Kind           string   `json:"kind"`
			Debug          *bool    `json:"debug,omitempty"`
			DebugPlacement string   `json:"debug_placement,omitempty"`
			ToolCacheNames []string `json:"tool_cache_names,omitempty"`
		} `json:"retry"`
	}{}
	requestBody.Retry.Kind = cfg.Kind
	requestBody.Retry.Debug = cfg.Debug
	requestBody.Retry.DebugPlacement = cfg.DebugPlacement
	requestBody.Retry.ToolCacheNames = cfg.ToolCacheNames

	encodedBody, err := json.Marshal(requestBody)
	if err != nil {
		return RequestRetryResult{}, errors.Wrap(err, "unable to encode as JSON")
	}

	endpoint := "/mint/api/retries?" + params.Encode()
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(encodedBody))
	if err != nil {
		return RequestRetryResult{}, errors.Wrap(err, "unable to create new HTTP request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.RoundTrip(req)
	if err != nil {
		return RequestRetryResult{}, errors.Wrap(err, "HTTP request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnprocessableEntity {
		requestError := &RetryRequestError{}
		if err := json.NewDecoder(resp.Body).Decode(requestError); err != nil {
			return RequestRetryResult{}, errors.WrapSentinel(errors.Wrap(err, "unable to parse API response"), errors.ErrBadRequest)
		}
		if requestError.Message == "" {
			requestError.Message = "Unable to call RWX API - 422 Unprocessable Entity"
		}
		return RequestRetryResult{}, requestError
	}

	result := RequestRetryResult{}
	if err := decodeRetryResponseJSON(resp, &result); err != nil {
		return RequestRetryResult{}, err
	}
	return result, nil
}

func (t RetryTarget) queryParams() (url.Values, error) {
	if t.ID == "" {
		return nil, badRetryRequest("missing retry target ID")
	}

	key := ""
	switch t.Type {
	case "", RetryTargetInferred:
		key = "id"
	case RetryTargetRun:
		key = "run_id"
	case RetryTargetTask:
		key = "task_id"
	default:
		return nil, badRetryRequest(fmt.Sprintf("invalid retry target type %q", t.Type))
	}

	return url.Values{key: []string{t.ID}}, nil
}

func decodeRetryResponseJSON(resp *http.Response, result any) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return errors.Wrap(err, "unable to parse API response")
		}
		return nil
	}

	message := extractErrorMessage(resp.Body)
	if message == "" {
		message = fmt.Sprintf("Unable to call RWX API - %s", resp.Status)
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnprocessableEntity {
		return badRetryRequest(message)
	}
	return classifyHTTPStatusError(resp.StatusCode, message)
}

func badRetryRequest(message string) error {
	return errors.WrapSentinel(errors.New(message), errors.ErrBadRequest)
}
