package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/rwx-cloud/rwx/internal/errors"
)

type AttachDebugSessionConfig struct {
	TaskID string
	Name   string
}

type DebugSessionAttachmentError struct {
	Code   string
	TaskID string
	Name   string
}

func (e *DebugSessionAttachmentError) Error() string {
	switch e.Code {
	case "invalid_task":
		return fmt.Sprintf("task %q cannot host an attached SSH session", e.TaskID)
	case "vault_access_required":
		return "the access token cannot unlock every vault used by this task"
	case "task_not_found":
		return fmt.Sprintf("task %q was not found", e.TaskID)
	case "task_not_running":
		return fmt.Sprintf("task %q is no longer running", e.TaskID)
	case "duplicate_open_name":
		return fmt.Sprintf("an open debug session named %q already exists", e.Name)
	case "attachment_closed":
		return fmt.Sprintf("task %q no longer accepts attached SSH sessions", e.TaskID)
	case "task_server_error":
		return "unable to attach an SSH session through the task server"
	default:
		return e.Code
	}
}

func (e *DebugSessionAttachmentError) Unwrap() error {
	switch e.Code {
	case "vault_access_required":
		return errors.ErrUnauthenticated
	case "task_not_found":
		return errors.ErrNotFound
	case "task_not_running", "attachment_closed":
		return errors.ErrGone
	case "task_server_error":
		return errors.ErrInternalServerError
	default:
		return errors.ErrBadRequest
	}
}

func (c Client) AttachDebugSession(cfg AttachDebugSessionConfig) (DebugSessionSummary, error) {
	if cfg.TaskID == "" {
		return DebugSessionSummary{}, errors.WrapSentinel(errors.New("missing task ID"), errors.ErrBadRequest)
	}

	requestBody := struct {
		Name string `json:"name,omitempty"`
	}{Name: cfg.Name}
	encodedBody, err := json.Marshal(requestBody)
	if err != nil {
		return DebugSessionSummary{}, errors.Wrap(err, "unable to encode as JSON")
	}

	endpoint := fmt.Sprintf("/mint/api/tasks/%s/debug_sessions", url.PathEscape(cfg.TaskID))
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(encodedBody))
	if err != nil {
		return DebugSessionSummary{}, errors.Wrap(err, "unable to create new HTTP request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.RoundTrip(req)
	if err != nil {
		return DebugSessionSummary{}, errors.Wrap(err, "HTTP request failed")
	}
	defer resp.Body.Close()

	responseBody := struct {
		DebugSession DebugSessionSummary `json:"debug_session"`
		Error        string              `json:"error"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&responseBody); err != nil {
		if resp.StatusCode == http.StatusNotFound {
			return DebugSessionSummary{}, &DebugSessionAttachmentError{Code: "task_not_found", TaskID: cfg.TaskID, Name: cfg.Name}
		}
		return DebugSessionSummary{}, errors.Wrap(err, "unable to parse API response")
	}

	if resp.StatusCode == http.StatusAccepted {
		return responseBody.DebugSession, nil
	}

	code := responseBody.Error
	if code == "" && resp.StatusCode == http.StatusNotFound {
		code = "task_not_found"
	}
	if code != "" {
		return DebugSessionSummary{}, &DebugSessionAttachmentError{Code: code, TaskID: cfg.TaskID, Name: cfg.Name}
	}

	message := fmt.Sprintf("Unable to call RWX API - %s", resp.Status)
	return DebugSessionSummary{}, classifyHTTPStatusError(resp.StatusCode, message)
}
