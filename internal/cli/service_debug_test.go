package cli_test

import (
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	internalErrors "github.com/rwx-cloud/rwx/internal/errors"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestService_DebuggingTask(t *testing.T) {
	const (
		privateTestKey = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACDiyT6ht8Z2XBEJpLR4/xmNouq5KDdn5G++cUcTH4EhzwAAAJhIWxlBSFsZ
QQAAAAtzc2gtZWQyNTUxOQAAACDiyT6ht8Z2XBEJpLR4/xmNouq5KDdn5G++cUcTH4Ehzw
AAAEC6442PQKevgYgeT0SIu9zwlnEMl6MF59ZgM+i0ByMv4eLJPqG3xnZcEQmktHj/GY2i
6rkoN2fkb75xRxMfgSHPAAAAEG1pbnQgQ0xJIHRlc3RpbmcBAgMEBQ==
-----END OPENSSH PRIVATE KEY-----`
		publicTestKey = `ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOLJPqG3xnZcEQmktHj/GY2i6rkoN2fkb75xRxMfgSHP rwx CLI testing`
	)

	t.Run("when the task is debuggable", func(t *testing.T) {
		s := setupTest(t)

		agentAddress := fmt.Sprintf("%d.example.org:1234", 123456)
		connectedViaSSH := false
		fetchedConnectionInfo := false
		interactiveSSHSessionStarted := false
		runID := fmt.Sprintf("run-%d", 123456)

		debugConfig := cli.DebugTaskConfig{
			DebugKey: runID,
		}

		s.mockAPI.MockGetDebugConnectionInfo = func(cfg api.GetDebugConnectionInfoConfig) (api.DebugConnectionInfo, error) {
			require.Equal(t, runID, cfg.DebugKey)
			fetchedConnectionInfo = true
			return api.DebugConnectionInfo{
				Debuggable:     true,
				PrivateUserKey: privateTestKey,
				PublicHostKey:  publicTestKey,
				Address:        agentAddress,
			}, nil
		}

		s.mockSSH.MockConnect = func(addr string, cfg ssh.ClientConfig) error {
			require.Equal(t, agentAddress, addr)
			require.Equal(t, "rwx-cli", cfg.User)
			connectedViaSSH = true
			return nil
		}

		s.mockSSH.MockInteractiveSession = func() error {
			interactiveSSHSessionStarted = true
			return nil
		}

		err := s.service.DebugTask(debugConfig)
		require.NoError(t, err)

		require.True(t, fetchedConnectionInfo)

		require.True(t, connectedViaSSH)

		require.True(t, interactiveSSHSessionStarted)
	})

	t.Run("when the task isn't debuggable yet", func(t *testing.T) {
		s := setupTest(t)

		runID := fmt.Sprintf("run-%d", 123456)
		debugConfig := cli.DebugTaskConfig{
			DebugKey: runID,
		}

		s.mockAPI.MockGetDebugConnectionInfo = func(cfg api.GetDebugConnectionInfoConfig) (api.DebugConnectionInfo, error) {
			require.Equal(t, runID, cfg.DebugKey)
			return api.DebugConnectionInfo{Debuggable: false}, nil
		}

		err := s.service.DebugTask(debugConfig)

		require.True(t, errors.Is(err, internalErrors.ErrRetry))
	})

	t.Run("when multiple sessions require selection in a non-TTY", func(t *testing.T) {
		s := setupTest(t)
		s.mockAPI.MockGetDebugConnectionInfo = func(cfg api.GetDebugConnectionInfoConfig) (api.DebugConnectionInfo, error) {
			require.Equal(t, "task-123", cfg.DebugKey)
			require.Empty(t, cfg.Session)
			return api.DebugConnectionInfo{}, &api.DebugSessionSelectionError{
				DebugSessions: []api.DebugSessionSummary{
					{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Name: "shell", Status: "connectable"},
					{ID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Status: "connectable"},
				},
			}
		}

		err := s.service.DebugTask(cli.DebugTaskConfig{DebugKey: "task-123"})

		require.EqualError(t, err, `multiple debug sessions are connectable

Available sessions:
  shell
    ID: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    Status: connectable
  (unnamed)
    ID: bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
    Status: connectable

Choose a session and retry:
  rwx debug --session aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa task-123`)
	})

	t.Run("when multiple sessions require selection in a TTY", func(t *testing.T) {
		s := setupTestWithTTY(t)
		_, err := s.mockStdin.WriteString("2\n")
		require.NoError(t, err)

		calls := 0
		s.mockAPI.MockGetDebugConnectionInfo = func(cfg api.GetDebugConnectionInfoConfig) (api.DebugConnectionInfo, error) {
			calls++
			require.Equal(t, "task-123", cfg.DebugKey)
			if calls == 1 {
				require.Empty(t, cfg.Session)
				return api.DebugConnectionInfo{}, &api.DebugSessionSelectionError{
					DebugSessions: []api.DebugSessionSummary{
						{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Name: "shell", Status: "connectable"},
						{ID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Status: "connectable"},
					},
				}
			}

			require.Equal(t, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", cfg.Session)
			return api.DebugConnectionInfo{
				Debuggable:     true,
				PrivateUserKey: privateTestKey,
				PublicHostKey:  publicTestKey,
				Address:        "debug.example.org:22",
				Username:       cfg.Session,
			}, nil
		}
		s.mockSSH.MockConnect = func(addr string, cfg ssh.ClientConfig) error {
			require.Equal(t, "debug.example.org:22", addr)
			require.Equal(t, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", cfg.User)
			return nil
		}
		s.mockSSH.MockInteractiveSession = func() error { return nil }

		err = s.service.DebugTask(cli.DebugTaskConfig{DebugKey: "task-123"})

		require.NoError(t, err)
		require.Equal(t, 2, calls)
		require.Equal(t, `Select a debug session:
  1. shell (aaaaaaaa) — connectable
  2. bbbbbbbb — connectable

Enter a number (1-2): `, s.mockStdout.String())
	})
}
