package cli

import (
	"fmt"
	"time"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"

	"golang.org/x/crypto/ssh"
)

type DebugTaskConfig struct {
	DebugKey string
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

	connectionInfo, err := s.APIClient.GetDebugConnectionInfo(api.GetDebugConnectionInfoConfig{DebugKey: cfg.DebugKey})
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
		User:            rwxCLISSHUser,
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
