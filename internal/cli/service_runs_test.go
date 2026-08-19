package cli_test

import (
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/stretchr/testify/require"
)

func TestService_ListRuns(t *testing.T) {
	t.Run("passes filters through to the API and returns the result", func(t *testing.T) {
		setup := setupTest(t)

		next := "cursor-2"
		setup.mockAPI.MockListRuns = func(cfg api.ListRunsConfig) (*api.ListRunsResult, error) {
			require.Equal(t, []string{"rwx-cloud/cloud"}, cfg.RepositoryNames)
			require.Equal(t, []string{"main", "develop"}, cfg.Branches)
			require.Equal(t, []string{"succeeded", "failed"}, cfg.ResultStatuses)
			require.True(t, cfg.MyRuns)
			require.Equal(t, 10, cfg.Limit)
			require.Equal(t, "cursor-1", cfg.Cursor)
			return &api.ListRunsResult{
				Runs:       []api.RunSummary{{ID: "run-1"}},
				Pagination: api.ListRunsPagination{NextCursor: &next, Limit: 10},
			}, nil
		}

		result, err := setup.service.ListRuns(cli.ListRunsConfig{
			RepositoryNames: []string{"rwx-cloud/cloud"},
			Branches:        []string{"main", "develop"},
			ResultStatuses:  []string{"succeeded", "failed"},
			MyRuns:          true,
			Limit:           10,
			Cursor:          "cursor-1",
		})

		require.NoError(t, err)
		require.Len(t, result.Runs, 1)
		require.Equal(t, "run-1", result.Runs[0].ID)
		require.Equal(t, "cursor-2", *result.Pagination.NextCursor)
	})

	t.Run("propagates an API error", func(t *testing.T) {
		setup := setupTest(t)

		setup.mockAPI.MockListRuns = func(cfg api.ListRunsConfig) (*api.ListRunsResult, error) {
			return nil, errors.New("boom")
		}

		result, err := setup.service.ListRuns(cli.ListRunsConfig{})

		require.Nil(t, result)
		require.Error(t, err)
	})
}

func TestService_CancelRun(t *testing.T) {
	t.Run("passes the run ID through to the API with no scoped token", func(t *testing.T) {
		setup := setupTest(t)

		called := false
		setup.mockAPI.MockCancelRun = func(runID, scopedToken string) error {
			called = true
			require.Equal(t, "run-123", runID)
			require.Empty(t, scopedToken)
			return nil
		}

		result, err := setup.service.CancelRun(cli.CancelRunConfig{RunID: "run-123"})

		require.NoError(t, err)
		require.True(t, called)
		require.Equal(t, "run-123", result.RunID)
	})

	t.Run("returns an error when the run ID is missing", func(t *testing.T) {
		setup := setupTest(t)

		setup.mockAPI.MockCancelRun = func(runID, scopedToken string) error {
			t.Fatal("the API must not be called without a run ID")
			return nil
		}

		result, err := setup.service.CancelRun(cli.CancelRunConfig{})

		require.Nil(t, result)
		require.Error(t, err)
	})

	t.Run("returns an error when the API fails", func(t *testing.T) {
		setup := setupTest(t)

		setup.mockAPI.MockCancelRun = func(runID, scopedToken string) error {
			return errors.New("run is already finished")
		}

		result, err := setup.service.CancelRun(cli.CancelRunConfig{RunID: "run-123"})

		require.Nil(t, result)
		require.ErrorContains(t, err, "run is already finished")
	})
}
