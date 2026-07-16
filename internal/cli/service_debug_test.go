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
}
