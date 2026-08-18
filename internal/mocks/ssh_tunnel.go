package mocks

import (
	"github.com/rwx-cloud/rwx/internal/errors"
	rwxssh "github.com/rwx-cloud/rwx/internal/ssh"
)

type SSHTunnelManager struct {
	MockOpen     func(rwxssh.TunnelConfig) (rwxssh.TunnelResult, error)
	MockClose    func(rwxssh.TunnelCloseConfig) error
	MockCloseAll func(string, string) error
	MockIsReady  func(int) bool
}

func (m *SSHTunnelManager) Open(cfg rwxssh.TunnelConfig) (rwxssh.TunnelResult, error) {
	if m.MockOpen != nil {
		return m.MockOpen(cfg)
	}
	return rwxssh.TunnelResult{}, errors.New("MockOpen was not configured")
}

func (m *SSHTunnelManager) Close(cfg rwxssh.TunnelCloseConfig) error {
	if m.MockClose != nil {
		return m.MockClose(cfg)
	}
	return nil
}

func (m *SSHTunnelManager) CloseAll(runID, stateDirectory string) error {
	if m.MockCloseAll != nil {
		return m.MockCloseAll(runID, stateDirectory)
	}
	return nil
}

func (m *SSHTunnelManager) IsReady(localPort int) bool {
	if m.MockIsReady != nil {
		return m.MockIsReady(localPort)
	}
	return true
}
