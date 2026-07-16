package cli

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"

	"golang.org/x/crypto/ssh"
)

type DebugTaskConfig struct {
	DebugKey string
	Session  string
}

func (c DebugTaskConfig) Validate() error {
	if c.DebugKey == "" {
		return errors.New("you must specify a run ID, a task ID, or an RWX Cloud URL")
	}

	return nil
}

// DebugTask will connect to a running task over SSH. Key exchange is facilitated over the Cloud API.
func (s Service) DebugTask(cfg DebugTaskConfig) error {
	err := cfg.Validate()
	if err != nil {
		return errors.Wrap(err, "validation failed")
	}

	connectionInfo, err := s.getDebugConnectionInfo(cfg)
	if err != nil {
		return err
	}

	if !connectionInfo.Debuggable {
		return errors.Wrap(errors.ErrRetry, "The task or run is not in a debuggable state")
	}

	privateUserKey, err := ssh.ParsePrivateKey([]byte(connectionInfo.PrivateUserKey))
	if err != nil {
		return errors.Wrap(err, "unable to parse key material retrieved from Cloud API")
	}

	rawPublicHostKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(connectionInfo.PublicHostKey))
	if err != nil {
		return errors.Wrap(err, "unable to parse host key retrieved from Cloud API")
	}

	publicHostKey, err := ssh.ParsePublicKey(rawPublicHostKey.Marshal())
	if err != nil {
		return errors.Wrap(err, "unable to parse host key retrieved from Cloud API")
	}

	sshConfig := ssh.ClientConfig{
		User:            debugSSHUser(connectionInfo),
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(privateUserKey)},
		HostKeyCallback: ssh.FixedHostKey(publicHostKey),
		BannerCallback: func(message string) error {
			fmt.Println(message)
			return nil
		},
	}

	connectStart := time.Now()
	if err = s.SSHClient.Connect(connectionInfo.Address, sshConfig); err != nil {
		s.recordTelemetry("ssh.connect", map[string]any{
			"duration_ms": time.Since(connectStart).Milliseconds(),
			"success":     false,
		})
		return errors.WrapSentinel(fmt.Errorf("unable to establish SSH connection to remote host: %w", err), errors.ErrSSH)
	}
	s.recordTelemetry("ssh.connect", map[string]any{
		"duration_ms": time.Since(connectStart).Milliseconds(),
		"success":     true,
	})
	defer s.SSHClient.Close()

	cmdStart := time.Now()
	if err := s.SSHClient.InteractiveSession(); err != nil {
		exitCode := -1
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitStatus()
		}

		s.recordTelemetry("ssh.command", map[string]any{
			"duration_ms": time.Since(cmdStart).Milliseconds(),
			"exit_code":   exitCode,
			"interactive": true,
		})

		// 137 is the default exit code for SIGKILL. This happens if the agent is forcefully terminating
		// the SSH server due to a run or task cancellation.
		if exitCode == 137 {
			return errors.New("The task was cancelled. Please check the Web UI for further details.")
		}

		return errors.WrapSentinel(fmt.Errorf("unable to start interactive session on remote host: %w", err), errors.ErrSSH)
	}

	s.recordTelemetry("ssh.command", map[string]any{
		"duration_ms": time.Since(cmdStart).Milliseconds(),
		"exit_code":   0,
		"interactive": true,
	})

	return nil
}

func (s Service) getDebugConnectionInfo(cfg DebugTaskConfig) (api.DebugConnectionInfo, error) {
	connectionInfo, err := s.APIClient.GetDebugConnectionInfo(api.GetDebugConnectionInfoConfig{
		DebugKey: cfg.DebugKey,
		Session:  cfg.Session,
	})
	if err == nil {
		return connectionInfo, nil
	}

	var selectionErr *api.DebugSessionSelectionError
	if !errors.As(err, &selectionErr) {
		return api.DebugConnectionInfo{}, err
	}

	if !s.StdoutIsTTY || cfg.Session != "" {
		return api.DebugConnectionInfo{}, debugSessionSelectionError(cfg.DebugKey, selectionErr.DebugSessions)
	}

	selected, err := s.promptForDebugSession(selectionErr.DebugSessions)
	if err != nil {
		return api.DebugConnectionInfo{}, err
	}

	connectionInfo, err = s.APIClient.GetDebugConnectionInfo(api.GetDebugConnectionInfoConfig{
		DebugKey: cfg.DebugKey,
		Session:  selected.ID,
	})
	if err != nil {
		if errors.As(err, &selectionErr) {
			return api.DebugConnectionInfo{}, debugSessionSelectionError(cfg.DebugKey, selectionErr.DebugSessions)
		}
		return api.DebugConnectionInfo{}, err
	}

	return connectionInfo, nil
}

func (s Service) promptForDebugSession(sessions []api.DebugSessionSummary) (api.DebugSessionSummary, error) {
	fmt.Fprintln(s.Stdout, "Select a debug session:")
	for index, session := range sessions {
		fmt.Fprintf(s.Stdout, "  %d. %s — %s\n", index+1, debugSessionLabel(session), session.Status)
	}
	fmt.Fprintln(s.Stdout)
	fmt.Fprintf(s.Stdout, "Enter a number (1-%d): ", len(sessions))

	scanner := bufio.NewScanner(s.Stdin)
	if !scanner.Scan() {
		return api.DebugSessionSummary{}, errors.New("no debug session selected")
	}

	choice, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
	if err != nil || choice < 1 || choice > len(sessions) {
		return api.DebugSessionSummary{}, errors.Errorf("invalid debug session selection: %s", scanner.Text())
	}

	return sessions[choice-1], nil
}

func debugSessionSelectionError(debugKey string, sessions []api.DebugSessionSummary) error {
	var message strings.Builder
	message.WriteString("multiple debug sessions are connectable\n\nAvailable sessions:\n")
	for _, session := range sessions {
		name := session.Name
		if name == "" {
			name = "(unnamed)"
		}
		fmt.Fprintf(&message, "  %s\n    ID: %s\n    Status: %s\n", name, session.ID, session.Status)
	}
	if len(sessions) > 0 {
		fmt.Fprintf(&message, "\nChoose a session and retry:\n  rwx debug --session %s %s", sessions[0].ID, debugKey)
	}

	return errors.New(message.String())
}

func debugSessionLabel(session api.DebugSessionSummary) string {
	shortID := session.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	if session.Name == "" {
		return shortID
	}
	return fmt.Sprintf("%s (%s)", session.Name, shortID)
}

func debugSSHUser(connectionInfo api.DebugConnectionInfo) string {
	if connectionInfo.Username != "" {
		return connectionInfo.Username
	}
	return rwxCLISSHUser
}
