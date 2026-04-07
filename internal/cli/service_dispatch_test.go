package cli_test

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	internalErrors "github.com/rwx-cloud/rwx/internal/errors"
	"github.com/stretchr/testify/require"
)

func TestService_InitiatingDispatch(t *testing.T) {
	t.Run("with valid dispatch parameters", func(t *testing.T) {
		s := setupTest(t)

		originalParams := map[string]string{"key1": "value1", "key2": "value2"}
		var receivedParams map[string]string

		dispatchConfig := cli.InitiateDispatchConfig{
			DispatchKey: "test-dispatch-key",
			Params:      originalParams,
			Title:       "Test Dispatch",
			Ref:         "main",
		}

		s.mockAPI.MockInitiateDispatch = func(cfg api.InitiateDispatchConfig) (*api.InitiateDispatchResult, error) {
			require.Equal(t, dispatchConfig.DispatchKey, cfg.DispatchKey)
			require.Equal(t, originalParams, cfg.Params)
			require.Equal(t, dispatchConfig.Title, cfg.Title)
			require.Equal(t, dispatchConfig.Ref, cfg.Ref)
			receivedParams = cfg.Params
			return &api.InitiateDispatchResult{DispatchId: "12345"}, nil
		}

		s.mockAPI.MockGetDispatch = func(cfg api.GetDispatchConfig) (*api.GetDispatchResult, error) {
			require.Equal(t, "12345", cfg.DispatchId)
			return &api.GetDispatchResult{
				Status: "ready",
				Runs: []api.GetDispatchRun{
					{RunID: "run-123", RunUrl: "https://example.com/run-123"},
				},
			}, nil
		}

		dispatchResult, err := s.service.InitiateDispatch(dispatchConfig)
		require.NoError(t, err)
		require.Equal(t, originalParams, receivedParams)
		require.Equal(t, "12345", dispatchResult.DispatchId)
	})

	t.Run("with missing dispatch key", func(t *testing.T) {
		s := setupTest(t)

		dispatchConfig := cli.InitiateDispatchConfig{
			DispatchKey: "",
		}

		_, err := s.service.InitiateDispatch(dispatchConfig)
		require.Error(t, err)
		require.Contains(t, err.Error(), "a dispatch key must be provided")
	})
}

func TestService_GettingDispatch(t *testing.T) {
	t.Run("when the dispatch result is not ready", func(t *testing.T) {
		s := setupTest(t)

		dispatchConfig := cli.GetDispatchConfig{
			DispatchId: "12345",
		}

		s.mockAPI.MockGetDispatch = func(cfg api.GetDispatchConfig) (*api.GetDispatchResult, error) {
			return &api.GetDispatchResult{Status: "not_ready"}, nil
		}

		_, err := s.service.GetDispatch(dispatchConfig)
		require.Error(t, err)
		require.True(t, errors.Is(err, internalErrors.ErrRetry))
	})

	t.Run("when the dispatch result contains an error", func(t *testing.T) {
		s := setupTest(t)

		dispatchConfig := cli.GetDispatchConfig{
			DispatchId: "12345",
		}

		s.mockAPI.MockGetDispatch = func(cfg api.GetDispatchConfig) (*api.GetDispatchResult, error) {
			return &api.GetDispatchResult{Status: "error", Error: "dispatch failed"}, nil
		}

		_, err := s.service.GetDispatch(dispatchConfig)
		require.Error(t, err)
		require.Contains(t, err.Error(), "dispatch failed")
	})

	t.Run("when the dispatch result succeeds", func(t *testing.T) {
		s := setupTest(t)

		dispatchConfig := cli.GetDispatchConfig{
			DispatchId: "12345",
		}

		s.mockAPI.MockGetDispatch = func(cfg api.GetDispatchConfig) (*api.GetDispatchResult, error) {
			return &api.GetDispatchResult{
				Status: "ready",
				Runs: []api.GetDispatchRun{
					{RunID: "runid", RunUrl: "runurl"},
				},
			}, nil
		}

		runs, err := s.service.GetDispatch(dispatchConfig)
		require.NoError(t, err)
		require.Equal(t, "runid", runs[0].RunID)
		require.Equal(t, "runurl", runs[0].RunURL)
	})

	t.Run("when no runs are created", func(t *testing.T) {
		s := setupTest(t)

		dispatchConfig := cli.GetDispatchConfig{
			DispatchId: "12345",
		}

		s.mockAPI.MockGetDispatch = func(cfg api.GetDispatchConfig) (*api.GetDispatchResult, error) {
			return &api.GetDispatchResult{Status: "ready", Runs: []api.GetDispatchRun{}}, nil
		}

		_, err := s.service.GetDispatch(dispatchConfig)
		require.Error(t, err)
		require.Contains(t, err.Error(), "No runs were created as a result of this dispatch")
	})
}
