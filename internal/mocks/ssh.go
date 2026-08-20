package mocks

import (
	"io"

	"github.com/rwx-cloud/rwx/internal/errors"

	"golang.org/x/crypto/ssh"
)

type SSH struct {
	MockConnect                                  func(addr string, cfg ssh.ClientConfig) error
	MockInteractiveSession                       func() error
	MockExecuteCommand                           func(command string) (int, error)
	MockExecuteCommandWithStdin                  func(command string, stdin io.Reader) (int, error)
	MockExecuteCommandWithOutput                 func(command string) (int, string, error)
	MockExecuteCommandWithSeparateOutput         func(command string) (int, string, string, error)
	MockExecuteCommandWithStdinAndCombinedOutput func(command string, stdin io.Reader) (int, string, error)
}

func (s *SSH) Close() error {
	return nil
}

func (s *SSH) Connect(addr string, cfg ssh.ClientConfig) error {
	if s.MockConnect != nil {
		return s.MockConnect(addr, cfg)
	}

	return errors.New("MockConnect was not configured")
}

func (s *SSH) InteractiveSession() error {
	if s.MockInteractiveSession != nil {
		return s.MockInteractiveSession()
	}

	return errors.New("MockInteractiveSession was not configured")
}

func (s *SSH) ExecuteCommand(command string) (int, error) {
	if s.MockExecuteCommand != nil {
		return s.MockExecuteCommand(command)
	}

	return -1, errors.New("MockExecuteCommand was not configured")
}

func (s *SSH) ExecuteCommandWithStdin(command string, stdin io.Reader) (int, error) {
	if s.MockExecuteCommandWithStdin != nil {
		return s.MockExecuteCommandWithStdin(command, stdin)
	}

	return -1, errors.New("MockExecuteCommandWithStdin was not configured")
}

func (s *SSH) ExecuteCommandWithOutput(command string) (int, string, error) {
	if s.MockExecuteCommandWithOutput != nil {
		return s.MockExecuteCommandWithOutput(command)
	}

	return -1, "", errors.New("MockExecuteCommandWithOutput was not configured")
}

func (s *SSH) ExecuteCommandWithSeparateOutput(command string) (int, string, string, error) {
	if s.MockExecuteCommandWithSeparateOutput != nil {
		return s.MockExecuteCommandWithSeparateOutput(command)
	}

	return -1, "", "", errors.New("MockExecuteCommandWithSeparateOutput was not configured")
}

func (s *SSH) ExecuteCommandWithStdinAndCombinedOutput(command string, stdin io.Reader) (int, string, error) {
	if s.MockExecuteCommandWithStdinAndCombinedOutput != nil {
		return s.MockExecuteCommandWithStdinAndCombinedOutput(command, stdin)
	}

	return -1, "", errors.New("MockExecuteCommandWithStdinAndCombinedOutput was not configured")
}
