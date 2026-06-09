package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/mocks"
	"github.com/rwx-cloud/rwx/internal/telemetry"
	"github.com/stretchr/testify/require"
)

type testSetup struct {
	config     cli.Config
	service    cli.Service
	collector  *telemetry.Collector
	mockAPI    *mocks.API
	mockSSH    *mocks.SSH
	mockGit    *mocks.Git
	mockDocker *mocks.DockerClient
	mockStdin  *bytes.Buffer
	mockStdout *strings.Builder
	mockStderr *strings.Builder
	tmp        string
	originalWd string
}

// absConfig returns an absolute config file path rooted in the test's temp directory.
func (s *testSetup) absConfig(relPath string) string {
	return filepath.Join(s.tmp, relPath)
}

func setupTest(t *testing.T) *testSetup {
	setup := &testSetup{}

	var err error
	setup.tmp, err = os.MkdirTemp(os.TempDir(), "cli-service")
	require.NoError(t, err)

	setup.tmp, err = filepath.EvalSymlinks(setup.tmp)
	require.NoError(t, err)
	setup.originalWd, err = os.Getwd()
	require.NoError(t, err)

	err = os.Chdir(setup.tmp)
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(setup.tmp, ".rwx"), 0o755))
	setup.mockAPI = new(mocks.API)
	setup.mockSSH = new(mocks.SSH)
	setup.mockSSH.MockExecuteCommandWithOutput = func(command string) (int, string, error) {
		return 0, "", nil
	}
	setup.mockGit = &mocks.Git{
		MockIsInstalled:      true,
		MockIsInsideWorkTree: true,
		MockGetHead:          "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	setup.mockDocker = new(mocks.DockerClient)
	setup.collector = telemetry.NewCollector()
	setup.mockStdin = &bytes.Buffer{}
	setup.mockStdout = &strings.Builder{}
	setup.mockStderr = &strings.Builder{}

	setup.config = cli.Config{
		APIClient:          setup.mockAPI,
		SSHClient:          setup.mockSSH,
		GitClient:          setup.mockGit,
		DockerCLI:          setup.mockDocker,
		TelemetryCollector: setup.collector,
		Stdin:              setup.mockStdin,
		Stdout:             setup.mockStdout,
		StdoutIsTTY:        false,
		Stderr:             setup.mockStderr,
		StderrIsTTY:        false,
	}
	setup.service, err = cli.NewService(setup.config)
	require.NoError(t, err)
	t.Cleanup(func() {
		err := os.Chdir(setup.originalWd)
		require.NoError(t, err)
		err = os.RemoveAll(setup.tmp)
		require.NoError(t, err)
	})

	return setup
}

func setupTestWithTTY(t *testing.T) *testSetup {
	s := setupTest(t)
	s.config.StdoutIsTTY = true
	s.config.StderrIsTTY = true
	var err error
	s.service, err = cli.NewService(s.config)
	require.NoError(t, err)
	return s
}

// drainEvents returns all telemetry events collected so far and resets the queue.
func (s *testSetup) drainEvents() []telemetry.Event {
	return s.collector.Drain()
}

// findEvent returns the first telemetry event with the given name, or nil.
func findEvent(events []telemetry.Event, name string) *telemetry.Event {
	for _, e := range events {
		if e.Event == name {
			return &e
		}
	}
	return nil
}

// findEvents returns all telemetry events with the given name.
func findEvents(events []telemetry.Event, name string) []telemetry.Event {
	var result []telemetry.Event
	for _, e := range events {
		if e.Event == name {
			result = append(result, e)
		}
	}
	return result
}
