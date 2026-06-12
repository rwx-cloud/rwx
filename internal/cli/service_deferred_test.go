package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

// setupDeferredRunTest lays down a minimal valid run definition so InitiateRun
// reaches the API call, and returns the run config to pass to InitiateRun.
func setupDeferredRunTest(t *testing.T, s *testSetup) cli.InitiateRunConfig {
	t.Helper()

	s.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
		return api.DefaultBaseResult{Image: "ubuntu:24.04", Config: "rwx/base 1.0.0", Arch: "x86_64"}, nil
	}
	s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
		return &api.PackageVersionsResult{
			LatestMajor: make(map[string]string),
			LatestMinor: make(map[string]map[string]string),
		}, nil
	}

	fileContent := "tasks:\n  - key: foo\n    run: echo 'bar'\nbase:\n  image: ubuntu:24.04\n  config: rwx/base 1.0.0\n"

	workingDir := filepath.Join(s.tmp, "working")
	require.NoError(t, os.MkdirAll(workingDir, 0o755))
	require.NoError(t, os.Chdir(workingDir))
	require.NoError(t, os.MkdirAll(filepath.Join(s.tmp, ".mint"), 0o755))

	testFile := filepath.Join(s.tmp, ".mint", "test.yml")
	require.NoError(t, os.WriteFile(testFile, []byte(fileContent), 0o644))

	return cli.InitiateRunConfig{MintFilePath: testFile}
}

func TestService_InitiateRun_Deferred(t *testing.T) {
	t.Run("polls until the deferred run is created, then resolves to the real run", func(t *testing.T) {
		s := setupTest(t)
		runConfig := setupDeferredRunTest(t, s)

		s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			return &api.InitiateRunResult{
				Deferred:       true,
				DeferredRunID:  "deferred-123",
				PlaceholderURL: "https://cloud.rwx.com/mint/rwx/runs/pending/deferred-123",
				PollingURL:     "https://cloud.rwx.com/mint/api/deferred_runs/deferred-123",
			}, nil
		}

		shortBackoff := 1
		calls := 0
		var receivedPollingURL string
		s.mockAPI.MockDeferredRunStatus = func(pollingURL string) (api.DeferredRunStatusResult, error) {
			receivedPollingURL = pollingURL
			calls++
			if calls == 1 {
				return api.DeferredRunStatusResult{
					State:   api.DeferredRunStatePending,
					Polling: api.PollingResult{Completed: false, BackoffMs: &shortBackoff},
				}, nil
			}
			return api.DeferredRunStatusResult{
				State:   api.DeferredRunStateCreated,
				RunID:   "run-789",
				RunURL:  "https://cloud.rwx.com/mint/rwx/runs/run-789",
				Polling: api.PollingResult{Completed: true},
			}, nil
		}

		result, err := s.service.InitiateRun(runConfig)
		require.NoError(t, err)
		require.Equal(t, "run-789", result.RunID)
		require.Equal(t, "https://cloud.rwx.com/mint/rwx/runs/run-789", result.RunURL)
		require.Contains(t, result.Message, "Run is watchable at https://cloud.rwx.com/mint/rwx/runs/run-789")
		require.False(t, result.Deferred)
		require.Equal(t, 2, calls)
		require.Equal(t, "https://cloud.rwx.com/mint/api/deferred_runs/deferred-123", receivedPollingURL)

		// The placeholder URL is printed up front so a user who Ctrl+Cs can still open the page.
		require.Contains(t, s.mockStdout.String(), "https://cloud.rwx.com/mint/rwx/runs/pending/deferred-123")

		var deferredEvent bool
		for _, e := range s.drainEvents() {
			if e.Event == "run.deferred" {
				deferredEvent = true
				require.Equal(t, api.DeferredRunStateCreated, e.Props["state"])
			}
		}
		require.True(t, deferredEvent, "expected a run.deferred telemetry event")
	})

	t.Run("returns the failure reason when the deferred run expires", func(t *testing.T) {
		s := setupTest(t)
		runConfig := setupDeferredRunTest(t, s)

		s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			return &api.InitiateRunResult{
				Deferred:       true,
				DeferredRunID:  "deferred-123",
				PlaceholderURL: "https://cloud.rwx.com/mint/rwx/runs/pending/deferred-123",
				PollingURL:     "https://cloud.rwx.com/mint/api/deferred_runs/deferred-123",
			}, nil
		}
		s.mockAPI.MockDeferredRunStatus = func(pollingURL string) (api.DeferredRunStatusResult, error) {
			return api.DeferredRunStatusResult{
				State:         api.DeferredRunStateExpired,
				FailureReason: "no task server became available",
				Polling:       api.PollingResult{Completed: true},
			}, nil
		}

		result, err := s.service.InitiateRun(runConfig)
		require.Nil(t, result)
		require.ErrorContains(t, err, "no task server became available")
	})
}
