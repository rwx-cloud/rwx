package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
	rwxssh "github.com/rwx-cloud/rwx/internal/ssh"

	"golang.org/x/crypto/ssh"
)

const rwxCLISSHUser = "rwx-cli"

type sandboxConnection struct {
	client         SSHClient
	transport      rwxssh.Transport
	connectionInfo api.SandboxConnectionInfo
}

func (s Service) sandboxTransport(connInfo *api.SandboxConnectionInfo) (rwxssh.Transport, error) {
	if s.SandboxTransport == "wss" {
		return s.sandboxWebSocketTransport(connInfo)
	}
	return rwxssh.TCPTransport{Address: connInfo.Address, Timeout: 5 * time.Second}, nil
}

func (s Service) sandboxWebSocketTransport(connInfo *api.SandboxConnectionInfo) (rwxssh.Transport, error) {
	if connInfo.WebSocketURL == "" {
		return nil, fmt.Errorf("WSS transport is not available for this sandbox")
	}
	if !strings.HasPrefix(connInfo.WebSocketURL, "wss://") {
		return nil, fmt.Errorf("sandbox WebSocket endpoint must use wss://")
	}

	accessToken, err := s.APIClient.AccessToken()
	if err != nil {
		return nil, errors.Wrap(err, "unable to retrieve access token for WSS transport")
	}
	if accessToken == "" {
		return nil, fmt.Errorf("RWX access token required for WSS transport")
	}

	return rwxssh.WebSocketTransport{
		Endpoint:    connInfo.WebSocketURL,
		AccessToken: accessToken,
	}, nil
}

func (s Service) connectSSH(connInfo *api.SandboxConnectionInfo) (*sandboxConnection, error) {
	automatic := s.SandboxTransport != "ssh" && s.SandboxTransport != "wss"
	transport, err := s.sandboxTransport(connInfo)
	if err != nil {
		return nil, err
	}

	if automatic && s.TransportPreferences != nil {
		if s.ResetSandboxTransport {
			s.saveTransportPreference("")
		} else {
			preference, readErr := s.TransportPreferences.Get("connection-transport")
			if readErr != nil {
				fmt.Fprintf(s.Stderr, "Warning: unable to read connection transport preference: %v\n", readErr)
			} else if preference == "wss" {
				transport, err = s.sandboxWebSocketTransport(connInfo)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	sshConfig, err := sandboxSSHConfig(connInfo, transport)
	if err != nil {
		return nil, err
	}
	_, usingWSS := transport.(rwxssh.WebSocketTransport)
	initialTransport := "ssh"
	if usingWSS {
		initialTransport = "wss"
	}
	finalTransport := initialTransport
	fallbackAttempted := false

	connectStart := time.Now()
	client := s.NewSSHClient()
	err = client.Connect(connInfo.Address, sshConfig, transport)
	if automatic && !usingWSS && connInfo.WebSocketURL != "" && rwxssh.IsConnectionFailure(err) {
		firstErr := err
		fmt.Fprintln(s.Stderr, "Direct SSH connection failed; trying WSS.")
		transport, err = s.sandboxWebSocketTransport(connInfo)
		if err == nil {
			sshConfig.Timeout = 15 * time.Second
			fallbackAttempted = true
			finalTransport = "wss"
			err = client.Connect(connInfo.Address, sshConfig, transport)
		}
		if err != nil {
			err = fmt.Errorf("initial connection failed (%v); alternate transport failed: %w", firstErr, err)
		} else {
			s.saveTransportPreference("wss")
		}
	}
	s.recordTelemetry("ssh.connect", map[string]any{
		"duration_ms":        time.Since(connectStart).Milliseconds(),
		"success":            err == nil,
		"initial_transport":  initialTransport,
		"final_transport":    finalTransport,
		"fallback_attempted": fallbackAttempted,
	})
	if err != nil {
		return nil, errors.WrapSentinel(fmt.Errorf("unable to establish SSH connection to remote host: %w", err), errors.ErrSSH)
	}

	return &sandboxConnection{client: client, transport: transport, connectionInfo: *connInfo}, nil
}

// Reconnect with the operation's successful transport, even if its client has
// already been closed. Do not reselect from preferences or try another transport.
func (s Service) reconnectSSH(previous *sandboxConnection) (*sandboxConnection, error) {
	sshConfig, err := sandboxSSHConfig(&previous.connectionInfo, previous.transport)
	if err != nil {
		return nil, err
	}
	transportName := "ssh"
	if _, usingWSS := previous.transport.(rwxssh.WebSocketTransport); usingWSS {
		transportName = "wss"
	}

	connectStart := time.Now()
	client := s.NewSSHClient()
	err = client.Connect(previous.connectionInfo.Address, sshConfig, previous.transport)
	s.recordTelemetry("ssh.connect", map[string]any{
		"duration_ms":        time.Since(connectStart).Milliseconds(),
		"success":            err == nil,
		"initial_transport":  transportName,
		"final_transport":    transportName,
		"fallback_attempted": false,
	})
	if err != nil {
		return nil, errors.WrapSentinel(fmt.Errorf("unable to establish SSH connection to remote host: %w", err), errors.ErrSSH)
	}

	return &sandboxConnection{client: client, transport: previous.transport, connectionInfo: previous.connectionInfo}, nil
}

func sandboxSSHConfig(connInfo *api.SandboxConnectionInfo, transport rwxssh.Transport) (ssh.ClientConfig, error) {
	privateUserKey, err := ssh.ParsePrivateKey([]byte(connInfo.PrivateUserKey))
	if err != nil {
		return ssh.ClientConfig{}, errors.Wrap(err, "unable to parse key material retrieved from Cloud API")
	}

	rawPublicHostKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(connInfo.PublicHostKey))
	if err != nil {
		return ssh.ClientConfig{}, errors.Wrap(err, "unable to parse host key retrieved from Cloud API")
	}

	publicHostKey, err := ssh.ParsePublicKey(rawPublicHostKey.Marshal())
	if err != nil {
		return ssh.ClientConfig{}, errors.Wrap(err, "unable to parse host key retrieved from Cloud API")
	}

	sshConfig := ssh.ClientConfig{
		User:            rwxCLISSHUser,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(privateUserKey)},
		HostKeyCallback: ssh.FixedHostKey(publicHostKey),
		Timeout:         5 * time.Second,
	}
	if _, usingWSS := transport.(rwxssh.WebSocketTransport); usingWSS {
		sshConfig.Timeout = 15 * time.Second
	}
	return sshConfig, nil
}

func (s Service) saveTransportPreference(value string) {
	if s.TransportPreferences == nil {
		return
	}
	if err := s.TransportPreferences.Set("connection-transport", value); err != nil {
		fmt.Fprintf(s.Stderr, "Warning: unable to save connection transport preference: %v\n", err)
	}
}
