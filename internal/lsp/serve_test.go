package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/rwx-cloud/rwx/internal/telemetry"
	"github.com/stretchr/testify/require"
)

func TestFindNode_ReturnsErrorWhenNotOnPath(t *testing.T) {
	t.Setenv("PATH", "")

	_, _, err := findNode(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "node is required but was not found on PATH")
}

func TestFindNode_ReturnsErrorWhenVersionTooOld(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on POSIX shebang scripts")
	}
	dir := writeFakeNode(t, "v16.20.2")
	t.Setenv("PATH", dir)

	_, _, err := findNode(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "node 18+ is required")
	require.Contains(t, err.Error(), "v16.20.2")
	require.Contains(t, err.Error(), "https://nodejs.org")
	require.Contains(t, err.Error(), "npx -y -p node@18 -- rwx lint")
}

func TestFindNode_AcceptsNode18WithEOLWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on POSIX shebang scripts")
	}
	dir := writeFakeNode(t, "v18.0.0")
	t.Setenv("PATH", dir)

	resolved, warning, err := findNode(nil)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "node"), resolved)
	require.Contains(t, warning, "v18.0.0")
	require.Contains(t, warning, "end-of-life")
	require.Contains(t, warning, "https://nodejs.org/en/about/previous-releases")
}

func TestFindNode_AcceptsNode20WithEOLWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on POSIX shebang scripts")
	}
	dir := writeFakeNode(t, "v20.10.0")
	t.Setenv("PATH", dir)

	resolved, warning, err := findNode(nil)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "node"), resolved)
	require.Contains(t, warning, "v20.10.0")
	require.Contains(t, warning, "end-of-life")
}

func TestFindNode_AcceptsNode22WithoutWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on POSIX shebang scripts")
	}
	dir := writeFakeNode(t, "v22.4.1")
	t.Setenv("PATH", dir)

	resolved, warning, err := findNode(nil)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "node"), resolved)
	require.Empty(t, warning)
}

func TestFindNode_AcceptsNode24WithoutWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on POSIX shebang scripts")
	}
	dir := writeFakeNode(t, "v24.0.0")
	t.Setenv("PATH", dir)

	resolved, warning, err := findNode(nil)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "node"), resolved)
	require.Empty(t, warning)
}

func TestFindNode_ReturnsErrorOnUnparsableVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on POSIX shebang scripts")
	}
	dir := writeFakeNode(t, "not-a-version")
	t.Setenv("PATH", dir)

	_, _, err := findNode(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unable to parse node version")
}

func TestFindNode_RecordsTelemetry_OK(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on POSIX shebang scripts")
	}
	dir := writeFakeNode(t, "v22.4.1")
	t.Setenv("PATH", dir)

	collector := telemetry.NewCollector()
	_, _, err := findNode(collector)
	require.NoError(t, err)

	events := collector.Drain()
	require.Len(t, events, 1)
	require.Equal(t, "lsp.node_check", events[0].Event)
	require.Equal(t, "ok", events[0].Props["status"])
	require.Equal(t, "v22.4.1", events[0].Props["version"])
	require.Equal(t, 22, events[0].Props["major"])
}

func TestFindNode_RecordsTelemetry_EOLWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on POSIX shebang scripts")
	}
	dir := writeFakeNode(t, "v18.0.0")
	t.Setenv("PATH", dir)

	collector := telemetry.NewCollector()
	_, warning, err := findNode(collector)
	require.NoError(t, err)
	require.NotEmpty(t, warning)

	events := collector.Drain()
	require.Len(t, events, 1)
	require.Equal(t, "lsp.node_check", events[0].Event)
	require.Equal(t, "eol_warning", events[0].Props["status"])
	require.Equal(t, "v18.0.0", events[0].Props["version"])
	require.Equal(t, 18, events[0].Props["major"])
}

func TestFindNode_RecordsTelemetry_TooOld(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on POSIX shebang scripts")
	}
	dir := writeFakeNode(t, "v17.0.0")
	t.Setenv("PATH", dir)

	collector := telemetry.NewCollector()
	_, _, err := findNode(collector)
	require.Error(t, err)

	events := collector.Drain()
	require.Len(t, events, 1)
	require.Equal(t, "lsp.node_check", events[0].Event)
	require.Equal(t, "too_old", events[0].Props["status"])
	require.Equal(t, "v17.0.0", events[0].Props["version"])
	require.Equal(t, 17, events[0].Props["major"])
}

func TestFindNode_RecordsTelemetry_Missing(t *testing.T) {
	t.Setenv("PATH", "")

	collector := telemetry.NewCollector()
	_, _, err := findNode(collector)
	require.Error(t, err)

	events := collector.Drain()
	require.Len(t, events, 1)
	require.Equal(t, "lsp.node_check", events[0].Event)
	require.Equal(t, "missing", events[0].Props["status"])
}

func TestFindNode_RecordsTelemetry_Unparsable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on POSIX shebang scripts")
	}
	dir := writeFakeNode(t, "not-a-version")
	t.Setenv("PATH", dir)

	collector := telemetry.NewCollector()
	_, _, err := findNode(collector)
	require.Error(t, err)

	events := collector.Drain()
	require.Len(t, events, 1)
	require.Equal(t, "lsp.node_check", events[0].Event)
	require.Equal(t, "unparsable", events[0].Props["status"])
	require.Equal(t, "not-a-version", events[0].Props["version"])
}

func TestParseNodeMajor(t *testing.T) {
	cases := []struct {
		input   string
		major   int
		wantErr bool
	}{
		{"v18.0.0", 18, false},
		{"v22.4.1", 22, false},
		{"v16.20.2", 16, false},
		{"18.0.0", 18, false},
		{"v18.0.0\n", 18, false},
		{"", 0, true},
		{"v", 0, true},
		{"vfoo.bar", 0, true},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			major, err := parseNodeMajor(c.input)
			if c.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.major, major)
		})
	}
}

// writeFakeNode creates a temporary directory containing an executable `node`
// script that prints the supplied version string. It returns the directory so
// callers can prepend it to PATH.
func writeFakeNode(t *testing.T, version string) string {
	t.Helper()
	dir := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\necho %q\n", version)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "node"), []byte(script), 0o755))
	return dir
}

func TestBundleHash_ReturnsDeterministicNonEmptyHash(t *testing.T) {
	hash1, err := bundleHash()
	require.NoError(t, err)
	require.NotEmpty(t, hash1)

	hash2, err := bundleHash()
	require.NoError(t, err)
	require.Equal(t, hash1, hash2)
}

func TestCleanStaleBundles_RemovesOldVersions(t *testing.T) {
	parentDir := t.TempDir()

	// Create a "current" bundle directory and two stale ones
	currentName := "v1.0.0-currenthash"
	require.NoError(t, os.MkdirAll(filepath.Join(parentDir, currentName), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(parentDir, "v0.9.0-oldhash"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(parentDir, "v0.8.0-olderhash"), 0o755))

	cleanStaleBundles(parentDir, currentName)

	entries, err := os.ReadDir(parentDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, currentName, entries[0].Name())
}

func TestCleanStaleBundles_NoErrorOnMissingDir(t *testing.T) {
	// Should not panic or error when the parent directory doesn't exist
	cleanStaleBundles("/nonexistent/path", "current")
}

func TestEnsureBundle_ExtractsAndCachesBundle(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	serverJS, err := ensureBundle()
	require.NoError(t, err)
	require.Contains(t, serverJS, "server.js")

	// serverJS is at <cache-dir>/bundle/server.js — go up 3 levels
	cacheDir := filepath.Dir(filepath.Dir(serverJS))
	markerFile := filepath.Join(cacheDir, ".extracted")
	_, err = os.Stat(markerFile)
	require.NoError(t, err, "marker file should exist after extraction")

	// Second call should be a cache hit (no error, same result)
	serverJS2, err := ensureBundle()
	require.NoError(t, err)
	require.Equal(t, serverJS, serverJS2)
}
