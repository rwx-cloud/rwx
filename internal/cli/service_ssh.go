package cli

import (
	"fmt"
	"time"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

const defaultSSHSessionPollInterval = time.Second

type AttachSSHSessionConfig struct {
	TaskID       string
	Name         string
	PollInterval time.Duration
}

func (c AttachSSHSessionConfig) Validate() error {
	if c.TaskID == "" {
		return errors.New("you must specify a task ID")
	}
	return nil
}

func (s Service) AttachSSHSession(cfg AttachSSHSessionConfig) error {
	if err := cfg.Validate(); err != nil {
		return errors.Wrap(err, "validation failed")
	}

	session, err := s.APIClient.AttachDebugSession(api.AttachDebugSessionConfig{
		TaskID: cfg.TaskID,
		Name:   cfg.Name,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(s.Stdout, "Attached SSH session %s. The task will continue running.\n", debugSessionLabel(session))
	stopSpinner := Spin("Waiting for SSH session to be ready...", s.StdoutIsTTY, s.Stdout)

	pollInterval := cfg.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultSSHSessionPollInterval
	}

	for {
		connectionInfo, err := s.APIClient.GetDebugConnectionInfo(api.GetDebugConnectionInfoConfig{
			DebugKey: cfg.TaskID,
			Session:  session.ID,
		})
		if err == nil {
			stopSpinner()
			return s.connectDebugSession(connectionInfo)
		}

		if !debugSessionIsStarting(err) {
			stopSpinner()
			return err
		}

		time.Sleep(pollInterval)
	}
}

func debugSessionIsStarting(err error) bool {
	var notFoundErr *api.DebugSessionNotFoundError
	if errors.As(err, &notFoundErr) {
		return true
	}

	var notConnectableErr *api.DebugSessionNotConnectableError
	return errors.As(err, &notConnectableErr) && notConnectableErr.DebugSession.Status == "starting"
}
