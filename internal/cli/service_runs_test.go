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
