package ssh

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"al.essio.dev/pkg/shellescape"
	"github.com/coder/websocket"
)

type WebSocketTransport struct {
	Endpoint    string
	AccessToken string
}

func (t WebSocketTransport) OpenSSHOptions() ([]string, []string, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, nil, err
	}

	proxyCommand := shellescape.QuoteCommand([]string{executable, "sandbox-wss-proxy", t.Endpoint})
	// OpenSSH expands percent tokens before the shell processes quoting.
	proxyCommand = strings.ReplaceAll(proxyCommand, "%", "%%")

	args := []string{"-o", "ProxyCommand=" + proxyCommand}
	env := []string{"RWX_SANDBOX_ACCESS_TOKEN=" + t.AccessToken}

	return args, env, nil
}

// Dial opens a WebSocket connection, retaining normal HTTPS proxy
// and certificate verification through the default HTTP transport.
func (t WebSocketTransport) Dial() (net.Conn, error) {
	endpointURL, err := url.Parse(t.Endpoint)
	if err != nil ||
		endpointURL.Scheme != "wss" ||
		endpointURL.Host == "" ||
		endpointURL.User != nil ||
		endpointURL.RawQuery != "" ||
		endpointURL.Fragment != "" {
		return nil, fmt.Errorf("sandbox WebSocket endpoint must be a wss:// URL without credentials, query, or fragment")
	}

	if t.AccessToken == "" || strings.ContainsAny(t.AccessToken, " \t\r\n") {
		return nil, fmt.Errorf("RWX access token required for WSS transport")
	}

	client := *http.DefaultClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+t.AccessToken)

	dialContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(dialContext, t.Endpoint, &websocket.DialOptions{
		HTTPClient: &client,
		HTTPHeader: headers,
	})
	if err != nil {
		return nil, fmt.Errorf("connect sandbox WebSocket: %w", err)
	}

	stream := websocket.NetConn(context.Background(), conn, websocket.MessageBinary)
	conn.SetReadLimit(1 << 20)

	return stream, nil
}
