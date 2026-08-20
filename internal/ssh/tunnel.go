package ssh

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type TunnelConfig struct {
	Key               string
	RunID             string
	Address           string
	PrivateUserKey    string
	PublicHostKey     string
	LocalPort         int
	TargetPort        int
	Scheme            string
	StateDirectory    string
	RemoteDestination string
}

type TunnelResult struct {
	LocalPort int
	Scheme    string
}

type TunnelCloseConfig struct {
	Key            string
	RunID          string
	StateDirectory string
}

type TunnelManager interface {
	Open(TunnelConfig) (TunnelResult, error)
	Close(TunnelCloseConfig) error
	CloseAll(runID, stateDirectory string) error
	IsReady(localPort int) bool
}

type tunnelManager struct {
	binary string
}

type tunnelState struct {
	Key        string `json:"key"`
	RunID      string `json:"runId"`
	LocalPort  int    `json:"localPort"`
	TargetPort int    `json:"targetPort"`
	Scheme     string `json:"scheme"`
	SocketPath string `json:"socketPath"`
}

func NewTunnelManager() TunnelManager {
	return tunnelManager{binary: "ssh"}
}

func (m tunnelManager) IsReady(localPort int) bool {
	connection, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", localPort), 250*time.Millisecond)
	if err != nil {
		return false
	}
	defer connection.Close()
	if err := connection.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		return false
	}
	buffer := make([]byte, 1)
	bytesRead, err := connection.Read(buffer)
	if bytesRead > 0 {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func (m tunnelManager) Open(cfg TunnelConfig) (TunnelResult, error) {
	if cfg.Key == "" {
		return TunnelResult{}, fmt.Errorf("background process key is required")
	}
	if cfg.TargetPort < 1 || cfg.TargetPort > 65535 {
		return TunnelResult{}, fmt.Errorf("target port must be between 1 and 65535")
	}
	if cfg.LocalPort < 0 || cfg.LocalPort > 65535 {
		return TunnelResult{}, fmt.Errorf("local port must be between 1 and 65535")
	}

	hash := sha256.Sum256([]byte(cfg.StateDirectory + "\x00" + cfg.Key))
	keyHash := hex.EncodeToString(hash[:8])
	previewDir := filepath.Join(cfg.StateDirectory, keyHash)
	if err := os.MkdirAll(previewDir, 0o700); err != nil {
		return TunnelResult{}, fmt.Errorf("unable to create background tunnel state: %w", err)
	}

	statePath := filepath.Join(previewDir, "state.json")
	state := tunnelState{}
	if data, err := os.ReadFile(statePath); err == nil {
		_ = json.Unmarshal(data, &state)
	}
	if state.SocketPath != "" {
		_ = exec.Command(m.binary, "-F", "/dev/null", "-S", state.SocketPath, "-O", "exit", "rwx-preview").Run()
	}

	scheme := cfg.Scheme
	if scheme == "" && state.Key == cfg.Key && state.RunID == cfg.RunID {
		scheme = state.Scheme
	}
	if scheme == "" {
		scheme = "http"
	}
	if scheme != "http" && scheme != "https" {
		return TunnelResult{}, fmt.Errorf("scheme must be http or https")
	}

	localPort := cfg.LocalPort
	if localPort == 0 && state.Key == cfg.Key && state.RunID == cfg.RunID {
		localPort = state.LocalPort
	}
	if localPort != 0 && cfg.LocalPort == 0 {
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", localPort))
		if err != nil {
			localPort = 0
		} else {
			_ = listener.Close()
		}
	}
	if localPort == 0 {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return TunnelResult{}, fmt.Errorf("unable to allocate local background process port: %w", err)
		}
		localPort = listener.Addr().(*net.TCPAddr).Port
		if err := listener.Close(); err != nil {
			return TunnelResult{}, fmt.Errorf("unable to release allocated background process port: %w", err)
		}
	}

	host, port, err := net.SplitHostPort(cfg.Address)
	if err != nil {
		return TunnelResult{}, fmt.Errorf("unable to parse sandbox SSH address: %w", err)
	}
	keyFile, err := os.CreateTemp("", "rwx-preview-key-*")
	if err != nil {
		return TunnelResult{}, err
	}
	keyPath := keyFile.Name()
	_ = keyFile.Close()
	defer os.Remove(keyPath)
	if err := os.WriteFile(keyPath, []byte(cfg.PrivateUserKey), 0o600); err != nil {
		return TunnelResult{}, err
	}

	knownHostsFile, err := os.CreateTemp("", "rwx-preview-known-hosts-*")
	if err != nil {
		return TunnelResult{}, err
	}
	knownHostsPath := knownHostsFile.Name()
	_ = knownHostsFile.Close()
	defer os.Remove(knownHostsPath)

	alias := "rwx-preview-" + keyHash
	if err := os.WriteFile(knownHostsPath, []byte(fmt.Sprintf("%s %s\n", alias, strings.TrimSpace(cfg.PublicHostKey))), 0o600); err != nil {
		return TunnelResult{}, err
	}

	socketPath := filepath.Join(os.TempDir(), "rwx-preview-"+keyHash+".sock")
	_ = os.Remove(socketPath)
	remoteDestination := cfg.RemoteDestination
	if remoteDestination == "" {
		remoteDestination = "127.0.0.1"
	}
	forward := fmt.Sprintf("127.0.0.1:%d:%s:%d", localPort, remoteDestination, cfg.TargetPort)
	args := []string{
		"-F", "/dev/null",
		"-f", "-N", "-M",
		"-S", socketPath,
		"-i", keyPath,
		"-o", "BatchMode=yes",
		"-o", "IdentitiesOnly=yes",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "UserKnownHostsFile=" + knownHostsPath,
		"-o", "HostKeyAlias=" + alias,
		"-o", "HostName=" + host,
		"-p", port,
		"-L", forward,
		"rwx-cli@" + alias,
	}
	if output, err := exec.Command(m.binary, args...).CombinedOutput(); err != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			return TunnelResult{}, fmt.Errorf("unable to open SSH background process tunnel: %s", message)
		}
		return TunnelResult{}, fmt.Errorf("unable to open SSH background process tunnel: %w", err)
	}

	state = tunnelState{
		Key:        cfg.Key,
		RunID:      cfg.RunID,
		LocalPort:  localPort,
		TargetPort: cfg.TargetPort,
		Scheme:     scheme,
		SocketPath: socketPath,
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return TunnelResult{}, err
	}
	data = append(data, '\n')
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		_ = exec.Command(m.binary, "-F", "/dev/null", "-S", socketPath, "-O", "exit", "rwx-preview").Run()
		return TunnelResult{}, fmt.Errorf("unable to save background tunnel state: %w", err)
	}

	return TunnelResult{LocalPort: localPort, Scheme: scheme}, nil
}

func (m tunnelManager) Close(cfg TunnelCloseConfig) error {
	if cfg.Key == "" {
		return fmt.Errorf("background process key is required")
	}

	hash := sha256.Sum256([]byte(cfg.StateDirectory + "\x00" + cfg.Key))
	stateDirectory := filepath.Join(cfg.StateDirectory, hex.EncodeToString(hash[:8]))
	statePath := filepath.Join(stateDirectory, "state.json")
	data, err := os.ReadFile(statePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("unable to read background tunnel state: %w", err)
	}

	var state tunnelState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("unable to decode background tunnel state: %w", err)
	}
	if state.Key != cfg.Key || state.RunID != cfg.RunID {
		return nil
	}

	if state.SocketPath != "" {
		_ = exec.Command(m.binary, "-F", "/dev/null", "-S", state.SocketPath, "-O", "exit", "rwx-preview").Run()
		_ = os.Remove(state.SocketPath)
	}
	if err := os.Remove(statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("unable to remove background tunnel state: %w", err)
	}
	_ = os.Remove(stateDirectory)
	return nil
}

func (m tunnelManager) CloseAll(runID, stateDirectory string) error {
	entries, err := os.ReadDir(stateDirectory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("unable to list background tunnel state: %w", err)
	}

	var closeErrors []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		statePath := filepath.Join(stateDirectory, entry.Name(), "state.json")
		data, readErr := os.ReadFile(statePath)
		if readErr != nil {
			if !errors.Is(readErr, os.ErrNotExist) {
				closeErrors = append(closeErrors, readErr.Error())
			}
			continue
		}
		var state tunnelState
		if decodeErr := json.Unmarshal(data, &state); decodeErr != nil {
			closeErrors = append(closeErrors, decodeErr.Error())
			continue
		}
		if state.RunID != runID {
			continue
		}
		if closeErr := m.Close(TunnelCloseConfig{Key: state.Key, RunID: runID, StateDirectory: stateDirectory}); closeErr != nil {
			closeErrors = append(closeErrors, closeErr.Error())
		}
	}
	if len(closeErrors) > 0 {
		return fmt.Errorf("unable to close all background tunnels: %s", strings.Join(closeErrors, "; "))
	}
	return nil
}
