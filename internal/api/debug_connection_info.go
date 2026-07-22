package api

import (
	"fmt"

	"github.com/rwx-cloud/rwx/internal/errors"
)

type GetDebugConnectionInfoConfig struct {
	DebugKey string
	Session  string
}

type DebugConnectionInfo struct {
	Debuggable     bool
	Address        string
	PublicHostKey  string `json:"public_host_key"`
	PrivateUserKey string `json:"private_user_key"`
	Username       string `json:"username"`
}

type DebugSessionSummary struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type DebugConnectionInfoError struct {
	Error         string                `json:"error"`
	DebugSessions []DebugSessionSummary `json:"debug_sessions"`
	DebugSession  DebugSessionSummary   `json:"debug_session"`
}

type DebugSessionSelectionError struct {
	DebugSessions []DebugSessionSummary
}

func (e *DebugSessionSelectionError) Error() string {
	return "multiple debug sessions are ready for connection"
}

func (e *DebugSessionSelectionError) Unwrap() error {
	return errors.ErrBadRequest
}

type DebugSessionNotConnectableError struct {
	DebugSession DebugSessionSummary
}

func (e *DebugSessionNotConnectableError) Error() string {
	selector := e.DebugSession.ID
	if e.DebugSession.Name != "" {
		selector = e.DebugSession.Name
	}
	return fmt.Sprintf("debug session %q is %s", selector, e.DebugSession.Status)
}

func (e *DebugSessionNotConnectableError) Unwrap() error {
	return errors.ErrBadRequest
}

type DebugSessionRequiresTaskError struct{}

func (e *DebugSessionRequiresTaskError) Error() string {
	return "--session requires a task ID or task URL"
}

func (e *DebugSessionRequiresTaskError) Unwrap() error {
	return errors.ErrBadRequest
}

type DebugSessionNotFoundError struct {
	Selector string
}

func (e *DebugSessionNotFoundError) Error() string {
	return fmt.Sprintf("debug session %q was not found", e.Selector)
}

func (e *DebugSessionNotFoundError) Unwrap() error {
	return errors.ErrNotFound
}
