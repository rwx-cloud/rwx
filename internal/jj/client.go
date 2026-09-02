package jj

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type Client struct {
	Binary    string
	GitBinary string
	Dir       string
}

const (
	workingCopy      = "@"
	commitIDTemplate = `commit_id ++ "\n"`
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

func (c *Client) run(args ...string) (string, string, error) {
	return c.exec(nil, args)
}

func (c *Client) runStale(args ...string) (string, string, error) {
	return c.exec([]string{"--ignore-working-copy"}, args)
}

func (c *Client) resolve(revset, template string) (string, string, error) {
	return c.run("log", "-r", revset, "--no-graph", "-T", template)
}

func (c *Client) resolveStale(revset, template string) (string, string, error) {
	return c.runStale("log", "-r", revset, "--no-graph", "-T", template)
}

func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func toRevset(ref string) string {
	if ref == "HEAD" {
		return workingCopy
	}
	return ref
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

func (c *Client) GetHeadCommit() (string, error) {
	out, stderr, err := c.resolve(workingCopy, commitIDTemplate)
	if err != nil {
		msg := stderr
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("unable to resolve HEAD: %s", msg)
	}

	head := firstLine(out)
	if head == "" {
		return "", fmt.Errorf("unable to resolve HEAD")
	}

	return head, nil
}

// GetShortHead reports a change id, not a commit id: it keys sandbox sessions,
// and jj rewrites @ on every snapshot, so a commit-id key would strand a sandbox
// on every edit. Change ids survive the rewrite and still resolve as revsets,
// which is what the sandbox ancestry fallback needs.
func (c *Client) GetShortHead() string {
	out, _, err := c.resolve(workingCopy, `change_id.short(7) ++ "\n"`)
	if err != nil {
		return ""
	}
	return firstLine(out)
}

// IsAncestor takes revsets, so a GetShortHead change id still matches after @
// has been rewritten.
func (c *Client) IsAncestor(candidateSHA, headRef string) bool {
	if candidateSHA == "" || headRef == "" {
		return false
	}

	revset := fmt.Sprintf("(%s) & ::(%s)", toRevset(candidateSHA), toRevset(headRef))
	out, _, err := c.resolveStale(revset, commitIDTemplate)
	if err != nil {
		return false
	}

	return firstLine(out) != ""
}
