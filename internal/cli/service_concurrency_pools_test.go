package cli_test

import (
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/stretchr/testify/require"
)

func TestService_ListConcurrencyPools(t *testing.T) {
	setup := setupTest(t)
	setup.mockAPI.MockListConcurrencyPools = func(cfg api.ListConcurrencyPoolsConfig) (*api.ListConcurrencyPoolsResult, error) {
		require.Equal(t, "rwx", cfg.Search)
		require.Equal(t, 10, cfg.Limit)
		require.Equal(t, "cursor-1", cfg.Cursor)
		return &api.ListConcurrencyPoolsResult{ConcurrencyPools: []api.ConcurrencyPool{{ID: "pool-1"}}}, nil
	}

	result, err := setup.service.ListConcurrencyPools(cli.ListConcurrencyPoolsConfig{Search: "rwx", Limit: 10, Cursor: "cursor-1"})
	require.NoError(t, err)
	require.Equal(t, "pool-1", result.ConcurrencyPools[0].ID)
}

func TestService_ListConcurrencyPoolsPropagatesError(t *testing.T) {
	setup := setupTest(t)
	setup.mockAPI.MockListConcurrencyPools = func(cfg api.ListConcurrencyPoolsConfig) (*api.ListConcurrencyPoolsResult, error) {
		return nil, errors.New("list failed")
	}

	result, err := setup.service.ListConcurrencyPools(cli.ListConcurrencyPoolsConfig{})
	require.Nil(t, result)
	require.ErrorContains(t, err, "list failed")
}

func TestService_ShowConcurrencyPool(t *testing.T) {
	setup := setupTest(t)
	setup.mockAPI.MockShowConcurrencyPool = func(id string) (*api.ConcurrencyPool, error) {
		require.Equal(t, "pool/one", id)
		return &api.ConcurrencyPool{ID: id, RecentRuns: []api.ConcurrencyPoolRun{{ID: "run-1"}}}, nil
	}
	setup.mockAPI.MockListConcurrencyPoolQueue = func(cfg api.ListConcurrencyPoolQueueConfig) (*api.ListConcurrencyPoolQueueResult, error) {
		require.Equal(t, "pool/one", cfg.PoolID)
		require.Equal(t, 20, cfg.Limit)
		require.Equal(t, "cursor-1", cfg.Cursor)
		next := "cursor-2"
		return &api.ListConcurrencyPoolQueueResult{
			Leases:     []api.ConcurrencyPoolLease{{ID: "lease-1"}},
			Pagination: api.ConcurrencyPoolPagination{NextCursor: &next, Limit: 20},
		}, nil
	}

	result, err := setup.service.ShowConcurrencyPool(cli.ShowConcurrencyPoolConfig{PoolID: "pool/one", Limit: 20, Cursor: "cursor-1"})
	require.NoError(t, err)
	require.Equal(t, "pool/one", result.ConcurrencyPool.ID)
	require.Equal(t, "run-1", result.ConcurrencyPool.RecentRuns[0].ID)
	require.Equal(t, "lease-1", result.Queue[0].ID)
	require.Equal(t, "cursor-2", *result.Pagination.NextCursor)
}

func TestService_ShowConcurrencyPoolStopsOnErrors(t *testing.T) {
	t.Run("pool", func(t *testing.T) {
		setup := setupTest(t)
		setup.mockAPI.MockShowConcurrencyPool = func(id string) (*api.ConcurrencyPool, error) {
			return nil, errors.New("show failed")
		}
		setup.mockAPI.MockListConcurrencyPoolQueue = func(cfg api.ListConcurrencyPoolQueueConfig) (*api.ListConcurrencyPoolQueueResult, error) {
			t.Fatal("queue must not be fetched when show fails")
			return nil, nil
		}

		result, err := setup.service.ShowConcurrencyPool(cli.ShowConcurrencyPoolConfig{PoolID: "pool-1"})
		require.Nil(t, result)
		require.ErrorContains(t, err, "show failed")
	})

	t.Run("queue", func(t *testing.T) {
		setup := setupTest(t)
		setup.mockAPI.MockShowConcurrencyPool = func(id string) (*api.ConcurrencyPool, error) {
			return &api.ConcurrencyPool{ID: id}, nil
		}
		setup.mockAPI.MockListConcurrencyPoolQueue = func(cfg api.ListConcurrencyPoolQueueConfig) (*api.ListConcurrencyPoolQueueResult, error) {
			return nil, errors.New("queue failed")
		}

		result, err := setup.service.ShowConcurrencyPool(cli.ShowConcurrencyPoolConfig{PoolID: "pool-1"})
		require.Nil(t, result)
		require.ErrorContains(t, err, "queue failed")
	})
}
