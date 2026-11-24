package ssh

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"al.essio.dev/pkg/shellescape"
	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	cryptossh "golang.org/x/crypto/ssh"
)

func TestWebSocketOpenSSHOptionsPreserveLiteralArguments(t *testing.T) {
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		t.Skip("OpenSSH is not installed")
	}

	executable, err := os.Executable()
	require.NoError(t, err)

	transport := WebSocketTransport{
		Endpoint:    "wss://sandbox.example.com/task%2Fid/ssh",
		AccessToken: "test-token",
	}
	args, _, err := transport.OpenSSHOptions()
	require.NoError(t, err)

	// Print the helper arguments after OpenSSH expansion instead of opening a connection.
	argumentsPath := filepath.Join(t.TempDir(), "arguments")
	proxyCommand := strings.TrimPrefix(args[1], "ProxyCommand=")
	printArguments := "printf '%%s\\n' " + proxyCommand + " > " + shellescape.Quote(argumentsPath)
	args[1] = "ProxyCommand=sh -c " + shellescape.Quote(printArguments)
	args = append([]string{"-F", "/dev/null", "-o", "BatchMode=yes"}, args...)
	args = append(args, "unused.invalid")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	output, err := exec.CommandContext(ctx, sshPath, args...).CombinedOutput()
	require.Error(t, err) // The diagnostic proxy exits without an SSH handshake.
	require.NoError(t, ctx.Err())

	arguments, err := os.ReadFile(argumentsPath)
	require.NoError(t, err, "%s", output)
	require.Equal(t, executable+"\nsandbox-wss-proxy\n"+transport.Endpoint+"\n", string(arguments))
}

func TestWebSocketRejectsInsecureOrCredentialURLs(t *testing.T) {
	for _, endpoint := range []string{
		"ws://localhost/ssh",
		"https://localhost/ssh",
		"wss:///ssh",
		"wss://token@localhost/ssh",
		"wss://localhost/ssh?token=secret",
		"wss://localhost/ssh#secret",
	} {
		t.Run(endpoint, func(t *testing.T) {
			transport := WebSocketTransport{
				Endpoint:    endpoint,
				AccessToken: "test-token",
			}

			_, err := transport.Dial()
			require.ErrorContains(t, err, "must be a wss:// URL")
		})
	}
}

func TestWebSocketAuthenticatesAndCarriesBinarySSHBytes(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()

		stream := websocket.NetConn(r.Context(), conn, websocket.MessageBinary)
		_, _ = io.Copy(stream, stream)
	}))
	defer server.Close()

	oldClient := http.DefaultClient
	http.DefaultClient = server.Client()
	t.Cleanup(func() { http.DefaultClient = oldClient })

	endpoint := strings.Replace(server.URL, "https://", "wss://", 1) + "/task/ssh"
	transport := WebSocketTransport{Endpoint: endpoint}

	_, err := transport.Dial()
	require.ErrorContains(t, err, "access token required")

	transport.AccessToken = "invalid"
	_, err = transport.Dial()
	require.Error(t, err)

	transport.AccessToken = "test-token"
	stream, err := transport.Dial()
	require.NoError(t, err)
	defer stream.Close()

	payload := []byte{0, 255, 13, 10, 17, 128, 42}
	_, err = stream.Write(payload)
	require.NoError(t, err)

	result := make([]byte, len(payload))
	_, err = io.ReadFull(stream, result)
	require.NoError(t, err)
	require.Equal(t, payload, result)
}

func TestWebSocketDoesNotRedirectCredentials(t *testing.T) {
	var requests atomic.Int32
	destination := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer destination.Close()

	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()

	oldClient := http.DefaultClient
	http.DefaultClient = origin.Client()
	t.Cleanup(func() { http.DefaultClient = oldClient })

	transport := WebSocketTransport{
		Endpoint:    strings.Replace(origin.URL, "https://", "wss://", 1) + "/ssh",
		AccessToken: "secret-token",
	}

	_, err := transport.Dial()
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret-token")
	require.Zero(t, requests.Load())
}

func TestWebSocketRequiresTrustedTLSAndDoesNotFallBack(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()

	client := &Client{}
	transport := WebSocketTransport{
		Endpoint:    strings.Replace(server.URL, "https://", "wss://", 1) + "/ssh",
		AccessToken: "test-token",
	}

	err := client.Connect(server.Listener.Addr().String(), cryptossh.ClientConfig{}, transport)
	require.ErrorContains(t, err, "certificate signed by unknown authority")
}
