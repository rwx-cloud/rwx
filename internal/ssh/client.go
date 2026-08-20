package ssh

import (
	"bytes"
	"io"
	"os"

	"github.com/rwx-cloud/rwx/internal/errors"

	tsize "github.com/kopoli/go-terminal-size"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

type Client struct {
	*ssh.Client
}

func (c *Client) Connect(address string, config ssh.ClientConfig) (err error) {
	c.Client, err = ssh.Dial("tcp", address, &config)
	return
}

func (c *Client) Close() error {
	return c.Client.Close()
}

func (c *Client) InteractiveSession() error {
	session, err := c.Client.NewSession()
	if err != nil {
		return errors.Wrapf(err, "unable to start interactive debug session")
	}
	defer session.Close()

	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	terminalSize, err := tsize.GetSize()
	if err != nil {
		return errors.Wrapf(err, "unable to determine terminal size")
	}

	oldTermState, err := term.MakeRaw(int(os.Stdout.Fd()))
	if err != nil {
		return errors.Wrapf(err, "unable to switch terminal to raw mode. Is stdout a PTY?")
	}
	defer func() {
		_ = term.Restore(int(os.Stdout.Fd()), oldTermState)
	}()

	if err := session.RequestPty(os.Getenv("TERM"), terminalSize.Height, terminalSize.Width, nil); err != nil {
		return errors.Wrapf(err, "unable to start PTY")
	}

	sizeChangeNotification, err := tsize.NewSizeListener()
	if err != nil {
		return errors.Wrapf(err, "unable to listen to terminal size changes")
	}
	defer sizeChangeNotification.Close()

	go func() {
		for size := range sizeChangeNotification.Change {
			_ = session.WindowChange(size.Height, size.Width)
		}
	}()

	if err := session.Shell(); err != nil {
		return errors.Wrapf(err, "unable to start shell")
	}

	// This is blocking
	if err := session.Wait(); err != nil {
		return errors.Wrapf(err, "connection was unexpectedly closed")
	}

	return nil
}

// ExecuteCommand runs a command non-interactively over SSH.
//
// Return values:
//   - (0, nil)   = command succeeded with exit code 0
//   - (N, nil)   = command completed with non-zero exit code N
//   - (-1, err)  = SSH/connection error (command may not have run)
func (c *Client) ExecuteCommand(command string) (int, error) {
	session, err := c.Client.NewSession()
	if err != nil {
		return -1, errors.Wrap(err, "unable to create SSH session")
	}
	defer session.Close()

	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	err = session.Run(command)
	if err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return exitErr.ExitStatus(), nil
		}
		return -1, errors.Wrap(err, "SSH command execution failed")
	}
	return 0, nil
}

// ExecuteCommandWithStdin runs a command non-interactively over SSH, piping data to stdin.
//
// Return values:
//   - (0, nil)   = command succeeded with exit code 0
//   - (N, nil)   = command completed with non-zero exit code N
//   - (-1, err)  = SSH/connection error (command may not have run)
func (c *Client) ExecuteCommandWithStdin(command string, stdin io.Reader) (int, error) {
	session, err := c.Client.NewSession()
	if err != nil {
		return -1, errors.Wrap(err, "unable to create SSH session")
	}
	defer session.Close()

	session.Stdin = stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	err = session.Run(command)
	if err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return exitErr.ExitStatus(), nil
		}
		return -1, errors.Wrap(err, "SSH command execution failed")
	}
	return 0, nil
}

// ExecuteCommandWithOutput runs a command non-interactively over SSH and captures stdout.
//
// Return values:
//   - (0, output, nil)   = command succeeded with exit code 0
//   - (N, output, nil)   = command completed with non-zero exit code N
//   - (-1, "", err)      = SSH/connection error (command may not have run)
func (c *Client) ExecuteCommandWithOutput(command string) (int, string, error) {
	session, err := c.Client.NewSession()
	if err != nil {
		return -1, "", errors.Wrap(err, "unable to create SSH session")
	}
	defer session.Close()

	var stdout bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = os.Stderr

	err = session.Run(command)
	if err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return exitErr.ExitStatus(), stdout.String(), nil
		}
		return -1, "", errors.Wrap(err, "SSH command execution failed")
	}
	return 0, stdout.String(), nil
}

// ExecuteCommandWithSeparateOutput runs a command non-interactively over SSH
// and captures stdout and stderr independently.
//
// Return values:
//   - (0, stdout, stderr, nil)   = command succeeded with exit code 0
//   - (N, stdout, stderr, nil)   = command completed with non-zero exit code N
//   - (-1, "", "", err)        = SSH/connection error (command may not have run)
func (c *Client) ExecuteCommandWithSeparateOutput(command string) (int, string, string, error) {
	session, err := c.Client.NewSession()
	if err != nil {
		return -1, "", "", errors.Wrap(err, "unable to create SSH session")
	}
	defer session.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	err = session.Run(command)
	if err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return exitErr.ExitStatus(), stdout.String(), stderr.String(), nil
		}
		return -1, "", "", errors.Wrap(err, "SSH command execution failed")
	}
	return 0, stdout.String(), stderr.String(), nil
}

// ExecuteCommandWithStdinAndCombinedOutput runs a command non-interactively over SSH,
// piping data to stdin, and captures both stdout and stderr.
//
// Return values:
//   - (0, output, nil)   = command succeeded with exit code 0
//   - (N, output, nil)   = command completed with non-zero exit code N
//   - (-1, "", err)      = SSH/connection error (command may not have run)
func (c *Client) ExecuteCommandWithStdinAndCombinedOutput(command string, stdin io.Reader) (int, string, error) {
	session, err := c.Client.NewSession()
	if err != nil {
		return -1, "", errors.Wrap(err, "unable to create SSH session")
	}
	defer session.Close()

	var combined bytes.Buffer
	session.Stdin = stdin
	session.Stdout = &combined
	session.Stderr = &combined

	err = session.Run(command)
	if err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return exitErr.ExitStatus(), combined.String(), nil
		}
		return -1, "", errors.Wrap(err, "SSH command execution failed")
	}
	return 0, combined.String(), nil
}
