package cli_test

import (
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	internalErrors "github.com/rwx-cloud/rwx/internal/errors"
	"github.com/stretchr/testify/require"
)

func retryOptions() api.RetryOptions {
	return api.RetryOptions{
		Retryable: true,
		Kinds: []api.RetryKind{
			{Value: "standard", Label: "Standard retry", Description: "The task will run again."},
			{Value: "no-tool-cache", Label: "Retry without tool caches", Description: "Only affected tasks will run again."},
		},
		Debug: api.RetryDebugOptions{
			Supported:        true,
			Placements:       []string{"end", "start"},
			DefaultPlacement: "end",
		},
		ToolCaches: []api.RetryToolCache{
			{Name: "bundler", ScopedTaskKeys: []string{"test"}, UsageDescription: "Used by test"},
			{Name: "golang", ScopedTaskKeys: []string{"build", "test"}, UsageDescription: "Used by 2 tasks"},
		},
	}
}

func TestService_Retry(t *testing.T) {
	t.Run("prints available kinds when a non-TTY must make a selection", func(t *testing.T) {
		setup := setupTest(t)
		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			require.Equal(t, api.RetryTarget{ID: "target-123", Type: api.RetryTargetInferred}, target)
			return retryOptions(), nil
		}
		setup.mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
			t.Fatal("the API must not request a retry before a kind is selected")
			return api.RequestRetryResult{}, nil
		}

		result, err := setup.service.Retry(cli.RetryConfig{
			Target: api.RetryTarget{ID: "target-123", Type: api.RetryTargetInferred},
		})

		require.Nil(t, result)
		require.ErrorIs(t, err, internalErrors.ErrBadRequest)
		require.EqualError(t, err, `retry kind selection is required

Available retry kinds:
  standard - Standard retry
    The task will run again.
  no-tool-cache - Retry without tool caches
    Only affected tasks will run again.

Choose a kind and retry:
  rwx retry --kind standard target-123`)
	})

	t.Run("uses flag selections without prompting", func(t *testing.T) {
		setup := setupTest(t)
		debug := false
		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			return retryOptions(), nil
		}
		setup.mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
			require.Equal(t, api.RequestRetryConfig{
				Target: api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
				Kind:   "standard",
				Debug:  &debug,
			}, cfg)
			return api.RequestRetryResult{Status: "retry_requested", RunID: "run-123", TaskID: "task-123"}, nil
		}

		result, err := setup.service.Retry(cli.RetryConfig{
			Target: api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
			Kind:   "standard",
			Debug:  &debug,
		})

		require.NoError(t, err)
		require.Equal(t, &api.RequestRetryResult{Status: "retry_requested", RunID: "run-123", TaskID: "task-123"}, result)
		require.Empty(t, setup.mockStdout.String())
	})

	t.Run("prints available caches when a non-TTY must make a selection", func(t *testing.T) {
		setup := setupTest(t)
		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			return retryOptions(), nil
		}

		result, err := setup.service.Retry(cli.RetryConfig{
			Target: api.RetryTarget{ID: "run-123", Type: api.RetryTargetRun},
			Kind:   "no-tool-cache",
		})

		require.Nil(t, result)
		require.ErrorIs(t, err, internalErrors.ErrBadRequest)
		require.EqualError(t, err, `tool cache selection is required for retry kind "no-tool-cache"

Available tool caches:
  bundler
    Used by test
  golang
    Used by 2 tasks

Choose one or more tool caches and retry:
  rwx runs retry --kind no-tool-cache --without-tool-cache bundler run-123`)
	})

	t.Run("prompts for all choices in a TTY", func(t *testing.T) {
		setup := setupTestWithTTY(t)
		_, err := setup.mockStdin.WriteString("2\n1,2\ny\n2\n")
		require.NoError(t, err)

		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			return retryOptions(), nil
		}
		setup.mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
			require.Equal(t, "no-tool-cache", cfg.Kind)
			require.Equal(t, []string{"bundler", "golang"}, cfg.ToolCacheNames)
			require.NotNil(t, cfg.Debug)
			require.True(t, *cfg.Debug)
			require.Equal(t, "start", cfg.DebugPlacement)
			return api.RequestRetryResult{Status: "retry_requested", RunID: "run-123", TaskID: "task-123"}, nil
		}

		result, err := setup.service.Retry(cli.RetryConfig{
			Target: api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		output := setup.mockStdout.String()
		require.Contains(t, output, "Select a retry kind:")
		require.Contains(t, output, "2. Retry without tool caches (no-tool-cache)")
		require.Contains(t, output, "Select one or more tool caches:")
		require.Contains(t, output, "Open a breakpoint? [y/N]:")
		require.Contains(t, output, "Select breakpoint placement:")
	})

	t.Run("uses the default debug placement outside a TTY", func(t *testing.T) {
		setup := setupTest(t)
		debug := true
		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			return retryOptions(), nil
		}
		setup.mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
			require.Equal(t, "end", cfg.DebugPlacement)
			return api.RequestRetryResult{Status: "retry_requested", RunID: "run-123", TaskID: "task-123"}, nil
		}

		_, err := setup.service.Retry(cli.RetryConfig{
			Target: api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
			Kind:   "standard",
			Debug:  &debug,
		})

		require.NoError(t, err)
	})

	t.Run("reports unavailable targets before prompting", func(t *testing.T) {
		setup := setupTestWithTTY(t)
		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			return api.RetryOptions{
				Retryable:         false,
				UnavailableReason: "This task cannot be retried in its current state.",
			}, nil
		}

		result, err := setup.service.Retry(cli.RetryConfig{
			Target: api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
		})

		require.Nil(t, result)
		require.EqualError(t, err, "This task cannot be retried in its current state.")
		require.ErrorIs(t, err, internalErrors.ErrBadRequest)
		require.Empty(t, setup.mockStdout.String())
	})
}
