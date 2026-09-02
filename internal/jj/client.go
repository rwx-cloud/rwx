package jj

import (
	"bytes"
	"os/exec"
	"strings"
)

type Client struct {
	Binary    string
	GitBinary string
	Dir       string
}

const (
	workingCopy = "@"
)

func (c *Client) exec(globals, args []string) (string, string, error) {
	full := append([]string{"--no-pager", "--color=never", "--quiet"}, globals...)
	cmd := exec.Command(c.Binary, append(full, args...)...)
	cmd.Dir = c.Dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

func (c *Client) runStale(args ...string) (string, string, error) {
	return c.exec([]string{"--ignore-working-copy"}, args)
}

func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// The jj backend reaches the object store through git, so both are required.
func (c *Client) MissingDependency() string {
	if _, err := exec.LookPath(c.Binary); err != nil {
		return "jj"
	}
	if _, err := exec.LookPath(c.GitBinary); err != nil {
		return "git"
	}
	return ""
}

func (c *Client) IsInsideWorkTree() bool {
	return c.GetTopLevel() != ""
}

func (c *Client) GetTopLevel() string {
	out, _, err := c.runStale("root")
	if err != nil {
		return ""
	}
	return firstLine(out)
}
