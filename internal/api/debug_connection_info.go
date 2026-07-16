package api

import "fmt"

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
	return "multiple debug sessions are connectable"
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
