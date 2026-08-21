package ssh

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTunnelManagerOpenRefreshesAndReusesLocalPort(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "ssh.log")
	binary := filepath.Join(tmp, "ssh")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$RWX_TEST_SSH_LOG\"\n"
	require.NoError(t, os.WriteFile(binary, []byte(script), 0o700))
	t.Setenv("RWX_TEST_SSH_LOG", logPath)

	manager := tunnelManager{binary: binary}
	config := TunnelConfig{
		Key:            "web",
		RunID:          "run-1",
		Address:        "192.0.2.10:22",
		PrivateUserKey: "private key",
		PublicHostKey:  "ssh-ed25519 public-key",
		TargetPort:     3000,
		Scheme:         "https",
		StateDirectory: filepath.Join(tmp, "state"),
	}

	first, err := manager.Open(config)
	require.NoError(t, err)
	require.Greater(t, first.LocalPort, 0)
	require.Equal(t, "https", first.Scheme)

	config.Scheme = ""
	second, err := manager.Open(config)
	require.NoError(t, err)
	require.Equal(t, first.LocalPort, second.LocalPort)
	require.Equal(t, "https", second.Scheme)

	logData, err := os.ReadFile(logPath)
	require.NoError(t, err)
	invocations := strings.Split(strings.TrimSpace(string(logData)), "\n")
	require.Len(t, invocations, 3)
	require.Contains(t, invocations[0], "-L 127.0.0.1:")
	require.Contains(t, invocations[1], "-O exit")
	require.Contains(t, invocations[2], "-L 127.0.0.1:")
	require.Contains(t, invocations[2], ":127.0.0.1:3000")
}

func TestTunnelManagerValidatesPorts(t *testing.T) {
	manager := tunnelManager{binary: "ssh"}

	_, err := manager.Open(TunnelConfig{Key: "web", TargetPort: 0})
	require.EqualError(t, err, "target port must be between 1 and 65535")

	_, err = manager.Open(TunnelConfig{Key: "web", TargetPort: 3000, LocalPort: 70000})
	require.EqualError(t, err, "local port must be between 1 and 65535")
}

func TestTunnelManagerValidatesScheme(t *testing.T) {
	manager := tunnelManager{binary: "ssh"}
	_, err := manager.Open(TunnelConfig{
		Key: "web", RunID: "run-1", TargetPort: 3000, Scheme: "ftp", StateDirectory: t.TempDir(),
	})
	require.EqualError(t, err, "scheme must be http or https")
}

func TestTunnelManagerDoesNotReuseLocalPortAcrossRuns(t *testing.T) {
	tmp := t.TempDir()
	binary := filepath.Join(tmp, "ssh")
	require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0o700))
	manager := tunnelManager{binary: binary}
	config := TunnelConfig{
		Key: "web", RunID: "run-1", Address: "192.0.2.10:22",
		PrivateUserKey: "private key", PublicHostKey: "ssh-ed25519 public-key",
		TargetPort: 3000, StateDirectory: filepath.Join(tmp, "state"),
	}
	first, err := manager.Open(config)
	require.NoError(t, err)
	require.Equal(t, "http", first.Scheme)

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", first.LocalPort))
	require.NoError(t, err)
	defer listener.Close()
	config.RunID = "run-2"
	second, err := manager.Open(config)
	require.NoError(t, err)
	require.NotEqual(t, first.LocalPort, second.LocalPort)
	entries, err := os.ReadDir(config.StateDirectory)
	require.NoError(t, err)
	require.Len(t, entries, 2, "same-key tunnels from different runs should have independent state")
}

func TestTunnelManagerCloseMatchesRun(t *testing.T) {
	tmp := t.TempDir()
	binary := filepath.Join(tmp, "ssh")
	require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0o700))
	manager := tunnelManager{binary: binary}
	config := TunnelConfig{
		Key: "web", RunID: "run-1", Address: "192.0.2.10:22",
		PrivateUserKey: "private key", PublicHostKey: "ssh-ed25519 public-key",
		TargetPort: 3000, StateDirectory: filepath.Join(tmp, "state"),
	}
	_, err := manager.Open(config)
	require.NoError(t, err)

	entries, err := os.ReadDir(config.StateDirectory)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	statePath := filepath.Join(config.StateDirectory, entries[0].Name(), "state.json")
	require.FileExists(t, statePath)

	require.NoError(t, manager.Close(TunnelCloseConfig{Key: "web", RunID: "run-2", StateDirectory: config.StateDirectory}))
	require.FileExists(t, statePath)
	require.NoError(t, manager.Close(TunnelCloseConfig{Key: "web", RunID: "run-1", StateDirectory: config.StateDirectory}))
	require.NoFileExists(t, statePath)
}

func TestTunnelManagerClosePreservesStateWhenControlMasterShutdownFails(t *testing.T) {
	manager, config, statePath, state := openTunnelWithFailingShutdown(t)

	err := manager.Close(TunnelCloseConfig{Key: config.Key, RunID: config.RunID, StateDirectory: config.StateDirectory})

	require.ErrorContains(t, err, "control exit failed")
	require.FileExists(t, statePath)
	require.FileExists(t, state.SocketPath)
}

func TestTunnelManagerOpenPreservesStateWhenExistingControlMasterShutdownFails(t *testing.T) {
	manager, config, statePath, state := openTunnelWithFailingShutdown(t)

	_, err := manager.Open(config)

	require.ErrorContains(t, err, "unable to replace existing background tunnel")
	require.ErrorContains(t, err, "control exit failed")
	require.FileExists(t, statePath)
	require.FileExists(t, state.SocketPath)
}

func openTunnelWithFailingShutdown(t *testing.T) (tunnelManager, TunnelConfig, string, tunnelState) {
	t.Helper()
	tmp := t.TempDir()
	binary := filepath.Join(tmp, "ssh")
	script := "#!/bin/sh\ncase \"$*\" in *\"-O exit\"*) echo 'control exit failed' >&2; exit 1;; esac\nexit 0\n"
	require.NoError(t, os.WriteFile(binary, []byte(script), 0o700))
	manager := tunnelManager{binary: binary}
	config := TunnelConfig{
		Key: "web", RunID: "run-1", Address: "192.0.2.10:22",
		PrivateUserKey: "private key", PublicHostKey: "ssh-ed25519 public-key",
		TargetPort: 3000, StateDirectory: filepath.Join(tmp, "state"),
	}
	_, err := manager.Open(config)
	require.NoError(t, err)

	entries, err := os.ReadDir(config.StateDirectory)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	statePath := filepath.Join(config.StateDirectory, entries[0].Name(), "state.json")
	data, err := os.ReadFile(statePath)
	require.NoError(t, err)
	var state tunnelState
	require.NoError(t, json.Unmarshal(data, &state))
	require.NoError(t, os.WriteFile(state.SocketPath, []byte("control socket"), 0o600))
	t.Cleanup(func() { _ = os.Remove(state.SocketPath) })
	return manager, config, statePath, state
}

func TestTunnelManagerReadinessRequiresConnectionToStayOpen(t *testing.T) {
	manager := tunnelManager{}

	closedListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() {
		connection, acceptErr := closedListener.Accept()
		if acceptErr == nil {
			_ = connection.Close()
		}
	}()
	require.False(t, manager.IsReady(closedListener.Addr().(*net.TCPAddr).Port))
	require.NoError(t, closedListener.Close())

	openListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() {
		connection, acceptErr := openListener.Accept()
		if acceptErr == nil {
			time.Sleep(250 * time.Millisecond)
			_ = connection.Close()
		}
	}()
	require.True(t, manager.IsReady(openListener.Addr().(*net.TCPAddr).Port))
	require.NoError(t, openListener.Close())
}
