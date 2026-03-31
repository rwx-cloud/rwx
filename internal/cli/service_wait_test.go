package cli_test

import (
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestService_GetRunStatus(t *testing.T) {
	t.Run("returns result when run completes immediately", func(t *testing.T) {
		setup := setupTest(t)

		setup.mockAPI.MockRunStatus = func(cfg api.RunStatusConfig) (api.RunStatusResult, error) {
			require.Equal(t, "run-123", cfg.RunID)
			return api.RunStatusResult{
				Status:  &api.RunStatus{Result: "succeeded"},
				RunID:   "run-123",
				Polling: api.PollingResult{Completed: true},
			}, nil
		}

		result, err := setup.service.GetRunStatus(cli.GetRunStatusConfig{
			RunID: "run-123",
			Wait:  true,
			Json:  false,
		})

		require.NoError(t, err)
		require.Equal(t, "run-123", result.RunID)
		require.Equal(t, "succeeded", result.ResultStatus)
		require.True(t, result.Completed)
	})

	t.Run("passes fail_fast to the API client", func(t *testing.T) {
		setup := setupTest(t)

		setup.mockAPI.MockRunStatus = func(cfg api.RunStatusConfig) (api.RunStatusResult, error) {
			require.True(t, cfg.FailFast)
			return api.RunStatusResult{
				Status:  &api.RunStatus{Result: "failed"},
				RunID:   "run-123",
				Polling: api.PollingResult{Completed: true},
			}, nil
		}

		result, err := setup.service.GetRunStatus(cli.GetRunStatusConfig{
			RunID:    "run-123",
			Wait:     true,
			FailFast: true,
			Json:     false,
		})

		require.NoError(t, err)
		require.Equal(t, "failed", result.ResultStatus)
	})

	t.Run("polls until run completes with failure", func(t *testing.T) {
		setup := setupTest(t)

		callCount := 0
		backoffMs := 0
		setup.mockAPI.MockRunStatus = func(cfg api.RunStatusConfig) (api.RunStatusResult, error) {
			callCount++
			if callCount < 3 {
				return api.RunStatusResult{
					Status:  &api.RunStatus{Result: "in_progress"},
					RunID:   "run-456",
					Polling: api.PollingResult{Completed: false, BackoffMs: &backoffMs},
				}, nil
			}
			return api.RunStatusResult{
				Status:  &api.RunStatus{Result: "failed"},
				RunID:   "run-456",
				Polling: api.PollingResult{Completed: true},
			}, nil
		}

		result, err := setup.service.GetRunStatus(cli.GetRunStatusConfig{
			RunID: "run-456",
			Wait:  true,
			Json:  false,
		})

		require.NoError(t, err)
		require.Equal(t, 3, callCount)
		require.Equal(t, "run-456", result.RunID)
		require.Equal(t, "failed", result.ResultStatus)
		require.True(t, result.Completed)
	})

	t.Run("polls until run completes with success", func(t *testing.T) {
		setup := setupTest(t)

		callCount := 0
		backoffMs := 0
		setup.mockAPI.MockRunStatus = func(cfg api.RunStatusConfig) (api.RunStatusResult, error) {
			callCount++
			if callCount < 2 {
				return api.RunStatusResult{
					Status:  &api.RunStatus{Result: "in_progress"},
					RunID:   "run-789",
					Polling: api.PollingResult{Completed: false, BackoffMs: &backoffMs},
				}, nil
			}
			return api.RunStatusResult{
				Status:  &api.RunStatus{Result: "succeeded"},
				RunID:   "run-789",
				Polling: api.PollingResult{Completed: true},
			}, nil
		}

		result, err := setup.service.GetRunStatus(cli.GetRunStatusConfig{
			RunID: "run-789",
			Wait:  true,
			Json:  false,
		})

		require.NoError(t, err)
		require.Equal(t, 2, callCount)
		require.Equal(t, "run-789", result.RunID)
		require.Equal(t, "succeeded", result.ResultStatus)
		require.True(t, result.Completed)
	})

	t.Run("returns error when backoff is nil and polling not completed", func(t *testing.T) {
		setup := setupTest(t)

		setup.mockAPI.MockRunStatus = func(cfg api.RunStatusConfig) (api.RunStatusResult, error) {
			return api.RunStatusResult{
				Status:  &api.RunStatus{Result: "in_progress"},
				RunID:   "run-789",
				Polling: api.PollingResult{Completed: false, BackoffMs: nil},
			}, nil
		}

		_, err := setup.service.GetRunStatus(cli.GetRunStatusConfig{
			RunID: "run-789",
			Wait:  true,
			Json:  false,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "unable to wait for run")
	})

	t.Run("returns empty status when run not found", func(t *testing.T) {
		setup := setupTest(t)

		setup.mockAPI.MockRunStatus = func(cfg api.RunStatusConfig) (api.RunStatusResult, error) {
			return api.RunStatusResult{
				Status:  nil,
				Polling: api.PollingResult{Completed: true},
			}, nil
		}

		result, err := setup.service.GetRunStatus(cli.GetRunStatusConfig{
			RunID: "nonexistent",
			Wait:  true,
			Json:  false,
		})

		require.NoError(t, err)
		require.Equal(t, "nonexistent", result.RunID)
		require.Equal(t, "", result.ResultStatus)
	})

	t.Run("returns error when API call fails", func(t *testing.T) {
		setup := setupTest(t)

		setup.mockAPI.MockRunStatus = func(cfg api.RunStatusConfig) (api.RunStatusResult, error) {
			return api.RunStatusResult{}, api.ErrNotFound
		}

		_, err := setup.service.GetRunStatus(cli.GetRunStatusConfig{
			RunID: "run-123",
			Wait:  true,
			Json:  false,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "unable to get run status")
	})

	t.Run("resolves run by branch and repository name", func(t *testing.T) {
		setup := setupTest(t)

		setup.mockAPI.MockRunStatus = func(cfg api.RunStatusConfig) (api.RunStatusResult, error) {
			require.Equal(t, "", cfg.RunID)
			require.Equal(t, "main", cfg.BranchName)
			require.Equal(t, "cli", cfg.RepositoryName)
			return api.RunStatusResult{
				Status:  &api.RunStatus{Result: "succeeded"},
				RunID:   "resolved-run-id",
				RunURL:  "https://cloud.rwx.com/mint/org/runs/resolved-run-id",
				Polling: api.PollingResult{Completed: true},
			}, nil
		}

		result, err := setup.service.GetRunStatus(cli.GetRunStatusConfig{
			BranchName:     "main",
			RepositoryName: "cli",
			Wait:           false,
			Json:           false,
		})

		require.NoError(t, err)
		require.Equal(t, "resolved-run-id", result.RunID)
		require.Equal(t, "https://cloud.rwx.com/mint/org/runs/resolved-run-id", result.RunURL)
		require.Equal(t, "succeeded", result.ResultStatus)
		require.True(t, result.Completed)
	})

	t.Run("returns ErrNotFound when API returns 404 for branch lookup", func(t *testing.T) {
		setup := setupTest(t)

		setup.mockAPI.MockRunStatus = func(cfg api.RunStatusConfig) (api.RunStatusResult, error) {
			return api.RunStatusResult{}, api.ErrNotFound
		}

		_, err := setup.service.GetRunStatus(cli.GetRunStatusConfig{
			BranchName:     "no-runs-here",
			RepositoryName: "cli",
			Wait:           false,
			Json:           false,
		})

		require.Error(t, err)
		require.ErrorIs(t, err, api.ErrNotFound)
	})

	t.Run("returns empty run ID when no run found for branch", func(t *testing.T) {
		setup := setupTest(t)

		setup.mockAPI.MockRunStatus = func(cfg api.RunStatusConfig) (api.RunStatusResult, error) {
			return api.RunStatusResult{
				Status:  nil,
				RunID:   "",
				Polling: api.PollingResult{Completed: true},
			}, nil
		}

		result, err := setup.service.GetRunStatus(cli.GetRunStatusConfig{
			BranchName:     "no-runs-here",
			RepositoryName: "cli",
			Wait:           false,
			Json:           false,
		})

		require.NoError(t, err)
		require.Equal(t, "", result.RunID)
	})

	t.Run("polls with branch lookup until run completes when Wait is true", func(t *testing.T) {
		setup := setupTest(t)

		callCount := 0
		backoffMs := 0
		setup.mockAPI.MockRunStatus = func(cfg api.RunStatusConfig) (api.RunStatusResult, error) {
			require.Equal(t, "main", cfg.BranchName)
			require.Equal(t, "cli", cfg.RepositoryName)
			callCount++
			if callCount < 2 {
				return api.RunStatusResult{
					Status:  &api.RunStatus{Result: "no_result"},
					RunID:   "resolved-run-id",
					Polling: api.PollingResult{Completed: false, BackoffMs: &backoffMs},
				}, nil
			}
			return api.RunStatusResult{
				Status:  &api.RunStatus{Result: "succeeded"},
				RunID:   "resolved-run-id",
				Polling: api.PollingResult{Completed: true},
			}, nil
		}

		result, err := setup.service.GetRunStatus(cli.GetRunStatusConfig{
			BranchName:     "main",
			RepositoryName: "cli",
			Wait:           true,
			Json:           false,
		})

		require.NoError(t, err)
		require.Equal(t, 2, callCount)
		require.Equal(t, "resolved-run-id", result.RunID)
		require.Equal(t, "succeeded", result.ResultStatus)
		require.True(t, result.Completed)
	})

	t.Run("returns commit when API includes commit_sha", func(t *testing.T) {
		setup := setupTest(t)

		commitSHA := "abc123def456"
		setup.mockAPI.MockRunStatus = func(cfg api.RunStatusConfig) (api.RunStatusResult, error) {
			return api.RunStatusResult{
				Status:  &api.RunStatus{Result: "succeeded"},
				RunID:   "run-123",
				Commit:  &commitSHA,
				Polling: api.PollingResult{Completed: true},
			}, nil
		}

		result, err := setup.service.GetRunStatus(cli.GetRunStatusConfig{
			RunID: "run-123",
			Wait:  false,
			Json:  false,
		})

		require.NoError(t, err)
		require.Equal(t, "abc123def456", result.Commit)
	})

	t.Run("returns empty commit when API omits commit_sha", func(t *testing.T) {
		setup := setupTest(t)

		setup.mockAPI.MockRunStatus = func(cfg api.RunStatusConfig) (api.RunStatusResult, error) {
			return api.RunStatusResult{
				Status:  &api.RunStatus{Result: "succeeded"},
				RunID:   "run-123",
				Commit:  nil,
				Polling: api.PollingResult{Completed: true},
			}, nil
		}

		result, err := setup.service.GetRunStatus(cli.GetRunStatusConfig{
			RunID: "run-123",
			Wait:  false,
			Json:  false,
		})

		require.NoError(t, err)
		require.Equal(t, "", result.Commit)
	})

	t.Run("returns current status without waiting when Wait is false", func(t *testing.T) {
		setup := setupTest(t)

		callCount := 0
		setup.mockAPI.MockRunStatus = func(cfg api.RunStatusConfig) (api.RunStatusResult, error) {
			callCount++
			return api.RunStatusResult{
				Status:  &api.RunStatus{Result: "in_progress"},
				RunID:   "run-123",
				Polling: api.PollingResult{Completed: false},
			}, nil
		}

		result, err := setup.service.GetRunStatus(cli.GetRunStatusConfig{
			RunID: "run-123",
			Wait:  false,
			Json:  false,
		})

		require.NoError(t, err)
		require.Equal(t, 1, callCount)
		require.Equal(t, "run-123", result.RunID)
		require.Equal(t, "in_progress", result.ResultStatus)
		require.False(t, result.Completed)
	})
}
