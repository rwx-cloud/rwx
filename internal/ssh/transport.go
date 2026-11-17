package ssh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"time"
)

// Transport supplies the connection beneath SSH, either directly or through OpenSSH.
// OpenSSHOptions returns additional arguments and environment entries, not credentials in arguments.
type Transport interface {
	Dial() (net.Conn, error)
	OpenSSHOptions() (args []string, env []string, err error)
}

type TCPTransport struct {
	Address string
	Timeout time.Duration
}

func (t TCPTransport) Dial() (net.Conn, error) {
	return net.DialTimeout("tcp", t.Address, t.Timeout)
}

func (t TCPTransport) OpenSSHOptions() ([]string, []string, error) {
	if t.Timeout > 0 {
		seconds := (t.Timeout + time.Second - 1) / time.Second
		return []string{"-o", fmt.Sprintf("ConnectTimeout=%d", seconds)}, nil, nil
	}
	return nil, nil, nil
}

// IsConnectionFailure recognizes network failures only. Callers must use it
// before opening any SSH sessions, never to retry a command or a git push.
func IsConnectionFailure(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, os.ErrDeadlineExceeded) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ETIMEDOUT) ||
		errors.Is(err, syscall.ENETUNREACH) ||
		errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}
