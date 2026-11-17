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
	if s.SandboxTransport != "wss" {
		return rwxssh.TCPTransport{Address: connInfo.Address}, nil
	}

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
	transport, err := s.sandboxTransport(connInfo)
	if err != nil {
		return nil, err
	}

	privateUserKey, err := ssh.ParsePrivateKey([]byte(connInfo.PrivateUserKey))
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse key material retrieved from Cloud API")
	}

	rawPublicHostKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(connInfo.PublicHostKey))
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse host key retrieved from Cloud API")
	}

	publicHostKey, err := ssh.ParsePublicKey(rawPublicHostKey.Marshal())
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse host key retrieved from Cloud API")
	}

	sshConfig := ssh.ClientConfig{
		User:            rwxCLISSHUser,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(privateUserKey)},
		HostKeyCallback: ssh.FixedHostKey(publicHostKey),
	}

	connectStart := time.Now()
	client := s.NewSSHClient()
	if err = client.Connect(connInfo.Address, sshConfig, transport); err != nil {
		s.recordTelemetry("ssh.connect", map[string]any{
			"duration_ms": time.Since(connectStart).Milliseconds(),
			"success":     false,
		})
		return nil, errors.WrapSentinel(fmt.Errorf("unable to establish SSH connection to remote host: %w", err), errors.ErrSSH)
	}

	s.recordTelemetry("ssh.connect", map[string]any{
		"duration_ms": time.Since(connectStart).Milliseconds(),
		"success":     true,
	})

	return &sandboxConnection{client: client, transport: transport, connectionInfo: *connInfo}, nil
}
