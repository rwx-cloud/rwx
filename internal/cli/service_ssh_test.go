package cli_test

import (
	"testing"
	"time"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestService_AttachSSHSession(t *testing.T) {
	t.Run("attaches, waits for ingestion and readiness, then connects", func(t *testing.T) {
		s := setupTest(t)
		const sessionID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

		s.mockAPI.MockAttachDebugSession = func(cfg api.AttachDebugSessionConfig) (api.DebugSessionSummary, error) {
			require.Equal(t, "task-123", cfg.TaskID)
			require.Equal(t, "shell", cfg.Name)
			return api.DebugSessionSummary{ID: sessionID, Name: "shell", Status: "starting"}, nil
		}

		connectionLookups := 0
		s.mockAPI.MockGetDebugConnectionInfo = func(cfg api.GetDebugConnectionInfoConfig) (api.DebugConnectionInfo, error) {
			connectionLookups++
			require.Equal(t, "task-123", cfg.DebugKey)
			require.Equal(t, sessionID, cfg.Session)
			switch connectionLookups {
			case 1:
				return api.DebugConnectionInfo{}, &api.DebugSessionNotFoundError{Selector: sessionID}
			case 2:
				return api.DebugConnectionInfo{}, &api.DebugSessionNotConnectableError{
					DebugSession: api.DebugSessionSummary{ID: sessionID, Name: "shell", Status: "starting"},
				}
			default:
				return api.DebugConnectionInfo{
					Debuggable:     true,
					Address:        "debug.example.org:22",
					PrivateUserKey: sandboxPrivateTestKey,
					PublicHostKey:  sandboxPublicTestKey,
					Username:       sessionID,
				}, nil
			}
		}

		s.mockSSH.MockConnect = func(addr string, cfg ssh.ClientConfig) error {
			require.Equal(t, "debug.example.org:22", addr)
			require.Equal(t, sessionID, cfg.User)
			return nil
		}
		s.mockSSH.MockInteractiveSession = func() error { return nil }

		err := s.service.AttachSSHSession(cli.AttachSSHSessionConfig{
			TaskID:       "task-123",
			Name:         "shell",
			PollInterval: time.Millisecond,
		})

		require.NoError(t, err)
		require.Equal(t, 3, connectionLookups)
		require.Contains(t, s.mockStdout.String(), "Attached SSH session shell (aaaaaaaa). The task will continue running.\n")
		require.Contains(t, s.mockStdout.String(), "Waiting for SSH session to be ready...\n")
	})

	t.Run("stops waiting when the attached session ends", func(t *testing.T) {
		s := setupTest(t)
		const sessionID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

		s.mockAPI.MockAttachDebugSession = func(api.AttachDebugSessionConfig) (api.DebugSessionSummary, error) {
			return api.DebugSessionSummary{ID: sessionID, Status: "starting"}, nil
		}
		s.mockAPI.MockGetDebugConnectionInfo = func(api.GetDebugConnectionInfoConfig) (api.DebugConnectionInfo, error) {
			return api.DebugConnectionInfo{}, &api.DebugSessionNotConnectableError{
				DebugSession: api.DebugSessionSummary{ID: sessionID, Status: "ended"},
			}
		}

		err := s.service.AttachSSHSession(cli.AttachSSHSessionConfig{
			TaskID:       "task-123",
			PollInterval: time.Millisecond,
		})

		require.EqualError(t, err, `debug session "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" is ended`)
	})

	t.Run("returns attachment errors without polling", func(t *testing.T) {
		s := setupTest(t)
		attachmentErr := &api.DebugSessionAttachmentError{Code: "attachment_closed", TaskID: "task-123"}
		s.mockAPI.MockAttachDebugSession = func(api.AttachDebugSessionConfig) (api.DebugSessionSummary, error) {
			return api.DebugSessionSummary{}, attachmentErr
		}
		s.mockAPI.MockGetDebugConnectionInfo = func(api.GetDebugConnectionInfoConfig) (api.DebugConnectionInfo, error) {
			t.Fatal("should not poll after attachment fails")
			return api.DebugConnectionInfo{}, nil
		}

		err := s.service.AttachSSHSession(cli.AttachSSHSessionConfig{TaskID: "task-123"})

		require.ErrorIs(t, err, attachmentErr)
	})
}
