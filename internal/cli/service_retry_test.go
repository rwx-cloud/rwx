package cli_test

import (
	"strings"
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

	t.Run("prints refreshed retry kinds after the endpoint rejects an explicit kind", func(t *testing.T) {
		setup := setupTest(t)
		initialOptions := retryOptions()
		initialOptions.Kinds = append([]api.RetryKind{{Value: "clean", Label: "Clean retry"}}, initialOptions.Kinds...)
		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			return initialOptions, nil
		}
		setup.mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
			return api.RequestRetryResult{}, &api.RetryRequestError{
				Message: "This retry configuration is not supported.",
				Errors: []api.RetryFieldError{{
					Field:   "kind",
					Message: "`clean` is not available for this task. Choose one of: `standard`, `no-tool-cache`.",
				}},
				Options: retryOptions(),
			}
		}

		result, err := setup.service.Retry(cli.RetryConfig{
			Target: api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
			Kind:   "clean",
		})

		require.Nil(t, result)
		require.ErrorIs(t, err, internalErrors.ErrBadRequest)
		require.EqualError(t, err, "This retry configuration is not supported.\n\n"+
			"Retry kind: `clean` is not available for this task. Choose one of: `standard`, `no-tool-cache`.\n\n"+
			"Available retry kinds:\n"+
			"  standard - Standard retry\n"+
			"    The task will run again.\n"+
			"  no-tool-cache - Retry without tool caches\n"+
			"    Only affected tasks will run again.\n\n"+
			"Try:\n"+
			"  rwx tasks retry task-123 --kind standard")
	})

	t.Run("prints refreshed tool caches after the endpoint rejects an explicit cache", func(t *testing.T) {
		setup := setupTest(t)
		initialOptions := retryOptions()
		initialOptions.ToolCaches = append(initialOptions.ToolCaches, api.RetryToolCache{Name: "obsolete"})
		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			return initialOptions, nil
		}
		setup.mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
			return api.RequestRetryResult{}, &api.RetryRequestError{
				Message: "This retry configuration is not supported.",
				Errors: []api.RetryFieldError{{
					Field:   "tool_cache_names",
					Message: "`obsolete` is not available for this retry target.",
				}},
				Options: retryOptions(),
			}
		}

		result, err := setup.service.Retry(cli.RetryConfig{
			Target:           api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
			Kind:             "no-tool-cache",
			WithoutToolCache: []string{"obsolete"},
		})

		require.Nil(t, result)
		require.EqualError(t, err, "This retry configuration is not supported.\n\n"+
			"Tool cache: `obsolete` is not available for this retry target.\n\n"+
			"Available tool caches:\n"+
			"  bundler\n"+
			"    Used by test\n"+
			"  golang\n"+
			"    Used by 2 tasks\n\n"+
			"Try:\n"+
			"  rwx tasks retry task-123 --kind no-tool-cache --without-tool-cache bundler")
	})

	t.Run("prints refreshed breakpoint placements after the endpoint rejects an explicit placement", func(t *testing.T) {
		setup := setupTest(t)
		initialOptions := retryOptions()
		initialOptions.Debug.Placements = append(initialOptions.Debug.Placements, "middle")
		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			return initialOptions, nil
		}
		setup.mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
			return api.RequestRetryResult{}, &api.RetryRequestError{
				Message: "This retry configuration is not supported.",
				Errors: []api.RetryFieldError{{
					Field:   "debug_placement",
					Message: "`middle` is not a valid breakpoint placement.",
				}},
				Options: retryOptions(),
			}
		}
		debug := true

		result, err := setup.service.Retry(cli.RetryConfig{
			Target:         api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
			Kind:           "standard",
			Debug:          &debug,
			DebugPlacement: "middle",
		})

		require.Nil(t, result)
		require.EqualError(t, err, "This retry configuration is not supported.\n\n"+
			"Breakpoint placement: `middle` is not a valid breakpoint placement.\n\n"+
			"Available breakpoint placements:\n"+
			"  end (default)\n"+
			"  start\n\n"+
			"Try:\n"+
			"  rwx tasks retry task-123 --kind standard --debug --debug-placement end")
	})

	t.Run("prompts once more with refreshed endpoint options for a bare TTY command", func(t *testing.T) {
		setup := setupTestWithTTY(t)
		_, err := setup.mockStdin.WriteString("1\n1\n")
		require.NoError(t, err)

		initialOptions := api.RetryOptions{
			Retryable: true,
			Kinds:     []api.RetryKind{{Value: "clean", Label: "Clean retry"}},
		}
		refreshedOptions := api.RetryOptions{
			Retryable: true,
			Kinds:     []api.RetryKind{{Value: "standard", Label: "Standard retry"}},
		}
		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			return initialOptions, nil
		}
		calls := 0
		setup.mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
			calls++
			if calls == 1 {
				require.Equal(t, "clean", cfg.Kind)
				return api.RequestRetryResult{}, &api.RetryRequestError{
					Message: "This retry configuration is not supported.",
					Errors:  []api.RetryFieldError{{Field: "kind", Message: "`clean` is no longer available."}},
					Options: refreshedOptions,
				}
			}

			require.Equal(t, "standard", cfg.Kind)
			return api.RequestRetryResult{Status: "retry_requested", RunID: "run-123", TaskID: "task-123"}, nil
		}

		result, err := setup.service.Retry(cli.RetryConfig{
			Target: api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
		})

		require.NoError(t, err)
		require.Equal(t, "task-123", result.TaskID)
		require.Equal(t, 2, calls)
		output := setup.mockStdout.String()
		require.Contains(t, output, "The available retry options changed.\n\n")
		require.Equal(t, 2, strings.Count(output, "Select a retry kind:"))
		require.Contains(t, output, "1. Standard retry (standard)")
	})

	t.Run("prints every refreshed option for an unknown endpoint field", func(t *testing.T) {
		setup := setupTest(t)
		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			return retryOptions(), nil
		}
		setup.mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
			return api.RequestRetryResult{}, &api.RetryRequestError{
				Message: "This retry configuration is not supported.",
				Errors:  []api.RetryFieldError{{Field: "target", Message: "The retry target changed."}},
				Options: retryOptions(),
			}
		}

		result, err := setup.service.Retry(cli.RetryConfig{
			Target: api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
			Kind:   "standard",
		})

		require.Nil(t, result)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Retry: The retry target changed.")
		require.NotContains(t, err.Error(), "target:")
		require.Contains(t, err.Error(), "Available retry kinds:")
		require.Contains(t, err.Error(), "Breakpoint:\n  Supported\n  Placements: end (default), start")
		require.Contains(t, err.Error(), "Available tool caches:")
		require.Contains(t, err.Error(), "Try:\n  rwx tasks retry task-123 --kind standard")
	})

	t.Run("does not replace an explicit TTY selection", func(t *testing.T) {
		setup := setupTestWithTTY(t)
		initialOptions := api.RetryOptions{
			Retryable: true,
			Kinds:     []api.RetryKind{{Value: "clean", Label: "Clean retry"}},
		}
		refreshedOptions := api.RetryOptions{
			Retryable: true,
			Kinds:     []api.RetryKind{{Value: "standard", Label: "Standard retry"}},
		}
		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			return initialOptions, nil
		}
		calls := 0
		setup.mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
			calls++
			return api.RequestRetryResult{}, &api.RetryRequestError{
				Message: "This retry configuration is not supported.",
				Errors:  []api.RetryFieldError{{Field: "kind", Message: "`clean` is no longer available."}},
				Options: refreshedOptions,
			}
		}

		result, err := setup.service.Retry(cli.RetryConfig{
			Target: api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
			Kind:   "clean",
		})

		require.Nil(t, result)
		require.Error(t, err)
		require.Equal(t, 1, calls)
		require.Empty(t, setup.mockStdout.String())
		require.Contains(t, err.Error(), "Available retry kinds:")
	})

	t.Run("stops after one retry with refreshed TTY options", func(t *testing.T) {
		setup := setupTestWithTTY(t)
		_, err := setup.mockStdin.WriteString("1\n1\n")
		require.NoError(t, err)

		initialOptions := api.RetryOptions{
			Retryable: true,
			Kinds:     []api.RetryKind{{Value: "clean", Label: "Clean retry"}},
		}
		refreshedOptions := api.RetryOptions{
			Retryable: true,
			Kinds:     []api.RetryKind{{Value: "standard", Label: "Standard retry"}},
		}
		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			return initialOptions, nil
		}
		calls := 0
		setup.mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
			calls++
			return api.RequestRetryResult{}, &api.RetryRequestError{
				Message: "This retry configuration is not supported.",
				Errors:  []api.RetryFieldError{{Field: "kind", Message: "The selected kind is no longer available."}},
				Options: refreshedOptions,
			}
		}

		result, err := setup.service.Retry(cli.RetryConfig{
			Target: api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
		})

		require.Nil(t, result)
		require.Error(t, err)
		require.Equal(t, 2, calls)
		require.Equal(t, 1, strings.Count(setup.mockStdout.String(), "The available retry options changed."))
	})

	t.Run("prints the breakpoint reason after the endpoint rejects debugging", func(t *testing.T) {
		setup := setupTest(t)
		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			return retryOptions(), nil
		}
		setup.mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
			options := retryOptions()
			options.Debug = api.RetryDebugOptions{
				Supported:      false,
				DisabledReason: "A breakpoint cannot be opened because vault access is required.",
			}
			return api.RequestRetryResult{}, &api.RetryRequestError{
				Message: "This retry configuration is not supported.",
				Errors: []api.RetryFieldError{{
					Field:   "debug",
					Message: options.Debug.DisabledReason,
				}},
				Options: options,
			}
		}
		debug := true

		result, err := setup.service.Retry(cli.RetryConfig{
			Target: api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
			Kind:   "standard",
			Debug:  &debug,
		})

		require.Nil(t, result)
		require.EqualError(t, err, "This retry configuration is not supported.\n\n"+
			"Breakpoint: A breakpoint cannot be opened because vault access is required.")
	})
}
