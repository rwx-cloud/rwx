package lsp

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/rwx-cloud/rwx/cmd/rwx/config"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/rwx-cloud/rwx/internal/telemetry"
)

// minNodeMajor is the minimum supported Node.js major version. The embedded
// language-server bundle is built with esbuild --target=node18, so anything
// older fails at runtime with confusing parser/syntax errors.
const minNodeMajor = 18

// recommendedNodeMajor is the lowest non-EOL Node major. Anything below this
// (but >= minNodeMajor) is allowed but produces a non-blocking warning.
const recommendedNodeMajor = 22

func Serve(collector *telemetry.Collector) (int, error) {
	nodePath, warning, err := findNode(collector)
	if err != nil {
		return 0, err
	}
	if warning != "" {
		fmt.Fprintln(os.Stderr, warning)
	}

	serverJS, err := ensureBundle()
	if err != nil {
		return 0, err
	}

	return runServer(nodePath, serverJS)
}

// findNode resolves the node binary on PATH and validates its major version.
// It returns the resolved path, an optional non-blocking warning (empty when
// none applies), and an error if node is missing, unparsable, or below
// minNodeMajor. An lsp.node_check telemetry event is recorded on every call.
func findNode(collector *telemetry.Collector) (string, string, error) {
	record := func(props map[string]any) {
		if collector == nil {
			return
		}
		collector.Record("lsp.node_check", props)
	}

	path, err := exec.LookPath("node")
	if err != nil {
		record(map[string]any{"status": "missing"})
		return "", "", errors.New("node is required but was not found on PATH. Install Node.js from https://nodejs.org")
	}

	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		record(map[string]any{"status": "version_check_failed"})
		return "", "", errors.Wrapf(err, "unable to determine node version by running %q --version", path)
	}

	version := strings.TrimSpace(string(out))
	major, err := parseNodeMajor(version)
	if err != nil {
		record(map[string]any{"status": "unparsable", "version": version})
		return "", "", errors.Wrapf(err, "unable to parse node version %q", version)
	}

	if major < minNodeMajor {
		record(map[string]any{"status": "too_old", "version": version, "major": major})
		return "", "", errors.Errorf(
			"node %d+ is required (found %s). Upgrade from https://nodejs.org, or run a one-off without upgrading via: npx -y -p node@%d -- rwx lint",
			minNodeMajor, version, minNodeMajor,
		)
	}

	var warning string
	status := "ok"
	if major < recommendedNodeMajor {
		warning = fmt.Sprintf(
			"warning: Node %s has reached end-of-life. See https://nodejs.org/en/about/previous-releases for more information.",
			version,
		)
		status = "eol_warning"
	}

	record(map[string]any{"status": status, "version": version, "major": major})
	return path, warning, nil
}

func parseNodeMajor(version string) (int, error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(version), "v")
	if trimmed == "" {
		return 0, errors.New("empty version string")
	}
	majorPart, _, _ := strings.Cut(trimmed, ".")
	major, err := strconv.Atoi(majorPart)
	if err != nil {
		return 0, errors.Wrapf(err, "major component %q is not an integer", majorPart)
	}
	return major, nil
}

func bundleHash() (string, error) {
	h := sha256.New()
	err := fs.WalkDir(bundle, "bundle", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := bundle.ReadFile(path)
		if err != nil {
			return err
		}
		h.Write([]byte(path))
		h.Write(data)
		return nil
	})
	if err != nil {
		return "", errors.Wrap(err, "unable to compute bundle hash")
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func ensureBundle() (string, error) {
	hash, err := bundleHash()
	if err != nil {
		return "", err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", errors.Wrap(err, "unable to determine home directory")
	}

	cacheDir := filepath.Join(homeDir, ".config", "rwx", "lsp-server", config.Version+"-"+hash)
	markerFile := filepath.Join(cacheDir, ".extracted")
	serverJS := filepath.Join(cacheDir, "bundle", "server.js")

	currentName := filepath.Base(cacheDir)
	parentDir := filepath.Dir(cacheDir)

	if _, err := os.Stat(markerFile); err == nil {
		return serverJS, nil
	}

	if err := os.RemoveAll(cacheDir); err != nil {
		return "", errors.Wrap(err, "unable to clean cache directory")
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", errors.Wrap(err, "unable to create cache directory")
	}

	err = fs.WalkDir(bundle, "bundle", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		targetPath := filepath.Join(cacheDir, path)

		if d.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}

		data, err := bundle.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(targetPath, data, 0o644)
	})
	if err != nil {
		_ = os.RemoveAll(cacheDir)
		return "", errors.Wrap(err, "unable to extract language server bundle")
	}

	if err := os.WriteFile(markerFile, nil, 0o644); err != nil {
		_ = os.RemoveAll(cacheDir)
		return "", errors.Wrap(err, "unable to write extraction marker")
	}

	cleanStaleBundles(parentDir, currentName)
	return serverJS, nil
}

func cleanStaleBundles(parentDir string, currentName string) {
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.Name() != currentName {
			_ = os.RemoveAll(filepath.Join(parentDir, entry.Name()))
		}
	}
}

func runServer(nodePath string, serverJS string) (int, error) {
	cmd := exec.Command(nodePath, serverJS, "--stdio")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return 0, errors.Wrap(err, "unable to start language server")
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for sig := range sigCh {
			_ = cmd.Process.Signal(sig)
		}
	}()

	err := cmd.Wait()
	signal.Stop(sigCh)
	close(sigCh)

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return 0, errors.Wrap(err, "language server exited unexpectedly")
	}

	return 0, nil
}
