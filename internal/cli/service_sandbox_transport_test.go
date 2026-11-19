package cli_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/config"
	rwxssh "github.com/rwx-cloud/rwx/internal/ssh"
	"github.com/rwx-cloud/rwx/internal/vcs"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// transportSandbox authenticates real SSH connections over TCP and WSS, but
// answers sandbox commands without running them against a real filesystem.
type transportSandbox struct {
	setup          *testSetup
	connectionInfo api.SandboxConnectionInfo
	wssServer      *httptest.Server
	preferences    *config.FileBackend
	signer         ssh.Signer

	// Tests change these while the server handles connections in goroutines.
	sshConnections    atomic.Int32
	wssRequests       atomic.Int32
	commandExecutions atomic.Int32
	rejectSSHAuth     atomic.Bool
	rejectWSSAccess   atomic.Bool
	disconnectCommand atomic.Bool
	missingCommit     atomic.Bool
}

func newTransportSandbox(t *testing.T) *transportSandbox {
	t.Helper()

	signer, err := ssh.ParsePrivateKey([]byte(sandboxPrivateTestKey))
	require.NoError(t, err)

	preferences, err := config.NewFileBackend([]string{t.TempDir()})
	require.NoError(t, err)

	sandbox := &transportSandbox{
		setup:       setupTest(t),
		preferences: preferences,
		signer:      signer,
	}

	// SSH starts unavailable. Tests can bind this address to make it available.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	sshAddress := listener.Addr().String()
	require.NoError(t, listener.Close())

	sandbox.wssServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sandbox.wssRequests.Add(1)
		if r.Header.Get("Authorization") != "Bearer test-token" || sandbox.rejectWSSAccess.Load() {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = ws.CloseNow() }()

		sandbox.serveSSH(websocket.NetConn(r.Context(), ws, websocket.MessageBinary))
	}))
	t.Cleanup(sandbox.wssServer.Close)

	// Trust this server's test certificate without disabling TLS verification.
	previousHTTPClient := http.DefaultClient
	http.DefaultClient = sandbox.wssServer.Client()
	t.Cleanup(func() { http.DefaultClient = previousHTTPClient })

	sandbox.connectionInfo = api.SandboxConnectionInfo{
		Sandboxable:    true,
		Address:        sshAddress,
		WebSocketURL:   strings.Replace(sandbox.wssServer.URL, "https://", "wss://", 1) + "/task/ssh",
		PrivateUserKey: sandboxPrivateTestKey,
		PublicHostKey:  sandboxPublicTestKey,
	}
	sandbox.setup.mockAPI.MockAccessToken = func() (string, error) {
		return "test-token", nil
	}
	sandbox.setup.mockAPI.MockGetSandboxConnectionInfo = func(string) (api.SandboxConnectionInfo, error) {
		return sandbox.connectionInfo, nil
	}
	sandbox.setup.service.NewSSHClient = func() cli.SSHClient {
		return &rwxssh.Client{}
	}
	sandbox.setup.service.TransportPreferences = preferences

	return sandbox
}

func (sandbox *transportSandbox) syncWithNewService(t *testing.T) error {
	t.Helper()

	service, err := cli.NewService(sandbox.setup.service.Config)
	require.NoError(t, err)
	sandbox.setup.service = service

	_, err = service.SyncSandbox(cli.SyncSandboxConfig{RunID: "run", Json: true})
	return err
}

func (sandbox *transportSandbox) listenSSH(t *testing.T, handler func(net.Conn)) {
	t.Helper()

	listener, err := net.Listen("tcp", sandbox.connectionInfo.Address)
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			sandbox.sshConnections.Add(1)
			go handler(conn)
		}
	}()
}

func (sandbox *transportSandbox) serveSSH(stream net.Conn) {
	defer stream.Close()

	serverConfig := ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if sandbox.rejectSSHAuth.Load() || !bytes.Equal(key.Marshal(), sandbox.signer.PublicKey().Marshal()) {
				return nil, fmt.Errorf("authentication rejected")
			}
			return nil, nil
		},
	}
	serverConfig.AddHostKey(sandbox.signer)

	conn, channels, requests, err := ssh.NewServerConn(stream, &serverConfig)
	if err != nil {
		return
	}
	defer conn.Close()
	go ssh.DiscardRequests(requests)

	for incomingChannel := range channels {
		channel, requests, err := incomingChannel.Accept()
		if err != nil {
			return
		}

		for request := range requests {
			if request.Type != "exec" {
				_ = request.Reply(false, nil)
				continue
			}

			var command struct{ Command string }
			if ssh.Unmarshal(request.Payload, &command) != nil {
				return
			}
			_ = request.Reply(true, nil)

			exitStatus := uint32(0)
			switch {
			case strings.Contains(command.Command, "git cat-file -e") && sandbox.missingCommit.Load():
				exitStatus = 1
			case strings.HasPrefix(command.Command, "__rwx_sandbox_process_status__ "):
				_, _ = io.WriteString(channel, `{"key":"web","status":"running"}`)
			case strings.HasPrefix(command.Command, "__rwx_sandbox_process_logs__ "):
				_, _ = io.WriteString(channel, `{"key":"web","status":"running","logPath":"/logs/web.log"}`)
			case command.Command == "tail -n +1 -F -- '/logs/web.log'":
				sandbox.commandExecutions.Add(1)
			case command.Command == "user-command":
				sandbox.commandExecutions.Add(1)
				if sandbox.disconnectCommand.Load() {
					return
				}
				exitStatus = 7
			}

			_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{exitStatus}))
			_ = channel.Close()
			break
		}
	}
}

func TestSandboxFallsBackToWSSBeforeSync(t *testing.T) {
	sandbox := newTransportSandbox(t)

	result, err := sandbox.setup.service.SyncSandbox(cli.SyncSandboxConfig{RunID: "run", Json: true})

	require.NoError(t, err)
	require.Equal(t, "run", result.RunID)
	require.EqualValues(t, 1, sandbox.wssRequests.Load())
	require.Equal(t, "Direct SSH connection failed; trying WSS.\n", sandbox.setup.mockStderr.String())

	events := findEvents(sandbox.setup.drainEvents(), "ssh.connect")
	require.Len(t, events, 1)
	require.Equal(t, "ssh", events[0].Props["initial_transport"])
	require.Equal(t, "wss", events[0].Props["final_transport"])
	require.Equal(t, true, events[0].Props["fallback_attempted"])
	require.Equal(t, true, events[0].Props["success"])
}

func TestSandboxRemembersWSSAcrossCommandsWithoutExpiry(t *testing.T) {
	sandbox := newTransportSandbox(t)
	require.NoError(t, sandbox.syncWithNewService(t))

	preference, err := sandbox.preferences.Get("connection-transport")
	require.NoError(t, err)
	require.Equal(t, "wss", preference)

	preferencePath := filepath.Join(sandbox.preferences.PrimaryDirectory, "connection-transport")
	yearsAgo := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(preferencePath, yearsAgo, yearsAgo))

	// Even with SSH available and a different sandbox URL, the next command
	// should use the saved WSS preference without announcing it again.
	sandbox.listenSSH(t, sandbox.serveSSH)
	sandbox.connectionInfo.WebSocketURL = strings.Replace(sandbox.connectionInfo.WebSocketURL, "/task/", "/another-task/", 1)
	sandbox.setup.mockStderr.Reset()
	sandbox.setup.drainEvents()

	require.NoError(t, sandbox.syncWithNewService(t))
	require.Zero(t, sandbox.sshConnections.Load())
	require.EqualValues(t, 2, sandbox.wssRequests.Load())
	require.Empty(t, sandbox.setup.mockStderr.String())

	events := findEvents(sandbox.setup.drainEvents(), "ssh.connect")
	require.Len(t, events, 1)
	require.Equal(t, "wss", events[0].Props["initial_transport"])
	require.Equal(t, "wss", events[0].Props["final_transport"])
	require.Equal(t, false, events[0].Props["fallback_attempted"])
	require.Equal(t, true, events[0].Props["success"])
}

func TestSandboxExplicitSSHLeavesSavedWSSUnchanged(t *testing.T) {
	sandbox := newTransportSandbox(t)
	require.NoError(t, sandbox.syncWithNewService(t))
	sandbox.listenSSH(t, sandbox.serveSSH)

	sandbox.setup.service.SandboxTransport = "ssh"
	require.NoError(t, sandbox.syncWithNewService(t))
	require.EqualValues(t, 1, sandbox.sshConnections.Load())

	sandbox.setup.service.SandboxTransport = "auto"
	require.NoError(t, sandbox.syncWithNewService(t))
	require.EqualValues(t, 1, sandbox.sshConnections.Load(), "explicit SSH must not overwrite the preference")
	require.EqualValues(t, 2, sandbox.wssRequests.Load())
}

func TestSandboxResetTransportClearsSavedWSS(t *testing.T) {
	sandbox := newTransportSandbox(t)
	require.NoError(t, sandbox.syncWithNewService(t))
	sandbox.listenSSH(t, sandbox.serveSSH)

	sandbox.setup.service.ResetSandboxTransport = true
	require.NoError(t, sandbox.syncWithNewService(t))
	require.EqualValues(t, 1, sandbox.sshConnections.Load())

	// Reset must clear the preference on disk, not just override it once.
	sandbox.setup.service.ResetSandboxTransport = false
	require.NoError(t, sandbox.syncWithNewService(t))
	require.EqualValues(t, 2, sandbox.sshConnections.Load())
	require.EqualValues(t, 1, sandbox.wssRequests.Load())
}

func TestSandboxPreferenceIsSharedAcrossCredentialsAndEndpoints(t *testing.T) {
	sandbox := newTransportSandbox(t)
	require.NoError(t, sandbox.syncWithNewService(t))
	sandbox.listenSSH(t, sandbox.serveSSH)

	sandbox.setup.mockAPI.MockAccessToken = func() (string, error) { return "other-token", nil }
	require.ErrorContains(t, sandbox.syncWithNewService(t), "401")
	require.Zero(t, sandbox.sshConnections.Load(), "new credentials must still use WSS and authenticate")
	require.EqualValues(t, 2, sandbox.wssRequests.Load())

	otherSandbox := newTransportSandbox(t)
	otherSandbox.setup.service.TransportPreferences = sandbox.preferences
	otherSandbox.listenSSH(t, otherSandbox.serveSSH)

	require.NoError(t, otherSandbox.syncWithNewService(t))
	require.Zero(t, otherSandbox.sshConnections.Load(), "another endpoint must reuse the saved preference")
	require.EqualValues(t, 1, otherSandbox.wssRequests.Load())
}

func TestSandboxRememberedWSSNetworkFailureNeverTriesSSH(t *testing.T) {
	sandbox := newTransportSandbox(t)
	require.NoError(t, sandbox.syncWithNewService(t))
	sandbox.listenSSH(t, sandbox.serveSSH)
	sandbox.wssServer.Close()
	sandbox.setup.drainEvents()

	require.ErrorContains(t, sandbox.syncWithNewService(t), "connection refused")
	require.Zero(t, sandbox.sshConnections.Load())

	events := findEvents(sandbox.setup.drainEvents(), "ssh.connect")
	require.Len(t, events, 1)
	require.Equal(t, "wss", events[0].Props["initial_transport"])
	require.Equal(t, "wss", events[0].Props["final_transport"])
	require.Equal(t, false, events[0].Props["fallback_attempted"])
	require.Equal(t, false, events[0].Props["success"])

	sandbox.setup.mockStderr.Reset()
	require.ErrorContains(t, sandbox.syncWithNewService(t), "connection refused")
	require.Empty(t, sandbox.setup.mockStderr.String())
	require.Zero(t, sandbox.sshConnections.Load(), "network failure must not erase the preference")

	sandbox.connectionInfo.WebSocketURL = ""
	require.ErrorContains(t, sandbox.syncWithNewService(t), "WSS transport is not available")
	require.Zero(t, sandbox.sshConnections.Load(), "missing WSS support must not silently select SSH")

	sandbox.setup.service.ResetSandboxTransport = true
	require.NoError(t, sandbox.syncWithNewService(t))
	require.EqualValues(t, 1, sandbox.sshConnections.Load(), "reset explicitly allows SSH again")
}

func TestSandboxExplicitSSHNeverFallsBack(t *testing.T) {
	sandbox := newTransportSandbox(t)
	sandbox.setup.service.SandboxTransport = "ssh"

	require.Error(t, sandbox.syncWithNewService(t))
	require.Zero(t, sandbox.wssRequests.Load())

	events := findEvents(sandbox.setup.drainEvents(), "ssh.connect")
	require.Len(t, events, 1)
	require.Equal(t, "ssh", events[0].Props["initial_transport"])
	require.Equal(t, "ssh", events[0].Props["final_transport"])
	require.Equal(t, false, events[0].Props["fallback_attempted"])
	require.Equal(t, false, events[0].Props["success"])
}

func TestSandboxExplicitWSSDoesNotSavePreference(t *testing.T) {
	sandbox := newTransportSandbox(t)
	sandbox.setup.service.SandboxTransport = "wss"

	require.NoError(t, sandbox.syncWithNewService(t))

	events := findEvents(sandbox.setup.drainEvents(), "ssh.connect")
	require.Len(t, events, 1)
	require.Equal(t, "wss", events[0].Props["initial_transport"])
	require.Equal(t, "wss", events[0].Props["final_transport"])
	require.Equal(t, false, events[0].Props["fallback_attempted"])
	require.Equal(t, true, events[0].Props["success"])

	sandbox.listenSSH(t, sandbox.serveSSH)
	sandbox.rejectWSSAccess.Store(true)
	require.ErrorContains(t, sandbox.syncWithNewService(t), "401")
	require.Zero(t, sandbox.sshConnections.Load(), "explicit WSS must not fall back to SSH")

	sandbox.setup.service.SandboxTransport = "auto"
	require.NoError(t, sandbox.syncWithNewService(t))
	require.EqualValues(t, 1, sandbox.sshConnections.Load(), "explicit WSS must not save a preference")
}

func TestSandboxAuthenticationFailureNeverFallsBack(t *testing.T) {
	sandbox := newTransportSandbox(t)
	sandbox.listenSSH(t, sandbox.serveSSH)
	sandbox.rejectSSHAuth.Store(true)

	require.ErrorContains(t, sandbox.syncWithNewService(t), "unable to authenticate")
	require.Zero(t, sandbox.wssRequests.Load())
}

func TestSandboxHostKeyFailureNeverFallsBack(t *testing.T) {
	sandbox := newTransportSandbox(t)
	sandbox.listenSSH(t, sandbox.serveSSH)

	wrongPublicKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	wrongHostKey, err := ssh.NewPublicKey(wrongPublicKey)
	require.NoError(t, err)
	sandbox.connectionInfo.PublicHostKey = string(ssh.MarshalAuthorizedKey(wrongHostKey))

	require.ErrorContains(t, sandbox.syncWithNewService(t), "host key mismatch")
	require.Zero(t, sandbox.wssRequests.Load())
}

func TestSandboxRememberedWSSSecurityFailuresNeverTrySSH(t *testing.T) {
	sandbox := newTransportSandbox(t)
	require.NoError(t, sandbox.syncWithNewService(t))
	sandbox.listenSSH(t, sandbox.serveSSH)

	sandbox.rejectWSSAccess.Store(true)
	require.ErrorContains(t, sandbox.syncWithNewService(t), "401")
	require.Zero(t, sandbox.sshConnections.Load())
	sandbox.rejectWSSAccess.Store(false)

	sandbox.rejectSSHAuth.Store(true)
	require.ErrorContains(t, sandbox.syncWithNewService(t), "unable to authenticate")
	require.Zero(t, sandbox.sshConnections.Load())
	sandbox.rejectSSHAuth.Store(false)

	http.DefaultClient = &http.Client{}
	require.ErrorContains(t, sandbox.syncWithNewService(t), "certificate signed by unknown authority")
	require.Zero(t, sandbox.sshConnections.Load())
	http.DefaultClient = sandbox.wssServer.Client()

	require.NoError(t, sandbox.syncWithNewService(t))
	require.Zero(t, sandbox.sshConnections.Load(), "security failures must not erase the preference")
}

func TestSandboxDoesNotRememberFailedWSSAuthentication(t *testing.T) {
	sandbox := newTransportSandbox(t)
	sandbox.rejectSSHAuth.Store(true)

	require.ErrorContains(t, sandbox.syncWithNewService(t), "unable to authenticate")

	events := findEvents(sandbox.setup.drainEvents(), "ssh.connect")
	require.Len(t, events, 1)
	require.Equal(t, "ssh", events[0].Props["initial_transport"])
	require.Equal(t, "wss", events[0].Props["final_transport"])
	require.Equal(t, true, events[0].Props["fallback_attempted"])
	require.Equal(t, false, events[0].Props["success"])

	sandbox.rejectSSHAuth.Store(false)
	sandbox.listenSSH(t, sandbox.serveSSH)
	require.NoError(t, sandbox.syncWithNewService(t))
	require.EqualValues(t, 1, sandbox.sshConnections.Load())
	require.EqualValues(t, 1, sandbox.wssRequests.Load())
}

func TestSandboxFallbackSetupFailureDoesNotRecordAnAttempt(t *testing.T) {
	sandbox := newTransportSandbox(t)
	sandbox.setup.mockAPI.MockAccessToken = func() (string, error) { return "", nil }

	require.ErrorContains(t, sandbox.syncWithNewService(t), "access token required for WSS")
	require.Zero(t, sandbox.wssRequests.Load())

	events := findEvents(sandbox.setup.drainEvents(), "ssh.connect")
	require.Len(t, events, 1)
	require.Equal(t, "ssh", events[0].Props["initial_transport"])
	require.Equal(t, "ssh", events[0].Props["final_transport"])
	require.Equal(t, false, events[0].Props["fallback_attempted"])
	require.Equal(t, false, events[0].Props["success"])
}

func TestSandboxFallsBackAfterSilentSSHHandshake(t *testing.T) {
	sandbox := newTransportSandbox(t)
	sandbox.listenSSH(t, func(conn net.Conn) {
		defer conn.Close()
		_, _ = io.Copy(io.Discard, conn)
	})

	start := time.Now()
	require.NoError(t, sandbox.syncWithNewService(t))
	fallbackDuration := time.Since(start)

	require.GreaterOrEqual(t, fallbackDuration, 5*time.Second)
	require.Less(t, fallbackDuration, 8*time.Second)
	require.EqualValues(t, 1, sandbox.wssRequests.Load())
	t.Logf("First sync including silent SSH fallback: %s", fallbackDuration)

	start = time.Now()
	require.NoError(t, sandbox.syncWithNewService(t))
	rememberedDuration := time.Since(start)

	require.Less(t, rememberedDuration, 3*time.Second)
	require.EqualValues(t, 1, sandbox.sshConnections.Load())
	t.Logf("Next sync using remembered WSS: %s", rememberedDuration)
}

func TestSandboxNeverReplaysCommandAfterDisconnect(t *testing.T) {
	sandbox := newTransportSandbox(t)
	sandbox.disconnectCommand.Store(true)

	_, err := sandbox.setup.service.ExecSandbox(cli.ExecSandboxConfig{
		RunID:   "run",
		Json:    true,
		Command: []string{"user-command"},
	})

	require.ErrorContains(t, err, "failed to execute command")
	require.EqualValues(t, 1, sandbox.commandExecutions.Load())
	require.EqualValues(t, 1, sandbox.wssRequests.Load())
}

func TestSandboxPreservesCommandExitCodeOverFallback(t *testing.T) {
	sandbox := newTransportSandbox(t)

	result, err := sandbox.setup.service.ExecSandbox(cli.ExecSandboxConfig{
		RunID:   "run",
		Json:    true,
		Command: []string{"user-command"},
	})

	require.NoError(t, err)
	require.Equal(t, 7, result.ExitCode)
	require.EqualValues(t, 1, sandbox.commandExecutions.Load())
	require.EqualValues(t, 1, sandbox.wssRequests.Load())
}

func TestSandboxGitPushUsesSelectedWSSAndDoesNotReplayFailure(t *testing.T) {
	sandbox := newTransportSandbox(t)
	sandbox.missingCommit.Store(true)

	pushes := 0
	sandbox.setup.mockVCS.MockPushRef = func(opts vcs.PushRefOptions) error {
		pushes++
		env := strings.Join(opts.Env, "\n")
		require.Contains(t, env, "sandbox-wss-proxy")
		require.Contains(t, env, sandbox.connectionInfo.WebSocketURL)
		require.Contains(t, env, "StrictHostKeyChecking=yes")
		return io.ErrUnexpectedEOF
	}

	require.ErrorContains(t, sandbox.syncWithNewService(t), "failed to push git commits")
	require.Equal(t, 1, pushes)
	require.EqualValues(t, 1, sandbox.wssRequests.Load())
}

func TestSandboxTunnelUsesSelectedWSS(t *testing.T) {
	sandbox := newTransportSandbox(t)
	sandbox.setup.mockTunnel.MockOpen = func(cfg rwxssh.TunnelConfig) (rwxssh.TunnelResult, error) {
		transport, ok := cfg.Transport.(rwxssh.WebSocketTransport)
		require.True(t, ok, "forwarding must reuse the authenticated connection's transport")
		require.Equal(t, sandbox.connectionInfo.WebSocketURL, transport.Endpoint)
		return rwxssh.TunnelResult{LocalPort: 12345, Scheme: "http"}, nil
	}

	result, err := sandbox.setup.service.TunnelSandbox(cli.TunnelSandboxConfig{
		RunID:      "run",
		Key:        "web",
		TargetPort: 3000,
		Json:       true,
	})

	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:12345", result.URL)
	require.EqualValues(t, 1, sandbox.wssRequests.Load())
}

func TestSandboxHandshakeDeadlineDoesNotLimitCommands(t *testing.T) {
	sandbox := newTransportSandbox(t)
	sandbox.listenSSH(t, sandbox.serveSSH)

	client := &rwxssh.Client{}
	clientConfig := ssh.ClientConfig{
		User:            "rwx-cli",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(sandbox.signer)},
		HostKeyCallback: ssh.FixedHostKey(sandbox.signer.PublicKey()),
		Timeout:         250 * time.Millisecond,
	}
	transport := rwxssh.TCPTransport{Address: sandbox.connectionInfo.Address}
	require.NoError(t, client.Connect(sandbox.connectionInfo.Address, clientConfig, transport))
	defer client.Close()

	time.Sleep(300 * time.Millisecond)
	exitCode, err := client.ExecuteCommand("user-command")

	require.NoError(t, err)
	require.Equal(t, 7, exitCode)
}

func TestSandboxLogReconnectKeepsFallbackWithoutSavedPreference(t *testing.T) {
	sandbox := newTransportSandbox(t)
	sandbox.setup.service.TransportPreferences = nil
	sandbox.listenSSH(t, func(conn net.Conn) { _ = conn.Close() })

	_, err := sandbox.setup.service.LogsSandboxBackground(cli.SandboxBackgroundLogsConfig{
		RunID:  "run",
		Name:   "web",
		Follow: true,
	})

	require.NoError(t, err)
	require.EqualValues(t, 1, sandbox.sshConnections.Load(), "reconnect must not retry direct SSH")
	require.EqualValues(t, 2, sandbox.wssRequests.Load())
	require.EqualValues(t, 1, sandbox.commandExecutions.Load(), "follow logs once on the reconnected WSS client")

	events := findEvents(sandbox.setup.drainEvents(), "ssh.connect")
	require.Len(t, events, 2)
	require.Equal(t, "wss", events[1].Props["initial_transport"])
	require.Equal(t, "wss", events[1].Props["final_transport"])
	require.Equal(t, false, events[1].Props["fallback_attempted"])
	require.Equal(t, true, events[1].Props["success"])
}
