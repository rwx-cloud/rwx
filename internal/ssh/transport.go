package ssh

import "net"

// Transport supplies the connection beneath SSH, either directly or through OpenSSH.
// OpenSSHOptions returns additional arguments and environment entries, not credentials in arguments.
type Transport interface {
	Dial() (net.Conn, error)
	OpenSSHOptions() (args []string, env []string, err error)
}

type TCPTransport struct {
	Address string
}

func (t TCPTransport) Dial() (net.Conn, error) {
	return net.Dial("tcp", t.Address)
}

func (t TCPTransport) OpenSSHOptions() ([]string, []string, error) {
	return nil, nil, nil
}
