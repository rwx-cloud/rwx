package main

import (
	"io"
	"os"

	"github.com/rwx-cloud/rwx/internal/ssh"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:    "sandbox-wss-proxy <wss-url>",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		// OpenSSH needs a clean byte stream, not normal CLI initialization/output.
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			return requireExperimentalSandboxAccess()
		},
		RunE: func(_ *cobra.Command, args []string) error {
			transport := ssh.WebSocketTransport{
				Endpoint:    args[0],
				AccessToken: os.Getenv("RWX_SANDBOX_ACCESS_TOKEN"),
			}

			conn, err := transport.Dial()
			if err != nil {
				return err
			}
			defer conn.Close()

			// OpenSSH -f must be able to return without this long-lived helper
			// holding its caller's captured stderr pipe open.
			_ = os.Stderr.Close()

			stdinDone := make(chan error, 1)
			go func() {
				_, err := io.Copy(conn, os.Stdin)
				stdinDone <- err
				_ = conn.Close()
			}()

			_, stdoutErr := io.Copy(os.Stdout, conn)

			select {
			case stdinErr := <-stdinDone:
				return stdinErr
			default:
				return stdoutErr
			}
		},
	})
}
