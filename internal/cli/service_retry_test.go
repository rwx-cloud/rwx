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
		Actions: []api.RetryAction{
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
	t.Run("uses the only retry action without prompting", func(t *testing.T) {
		setup := setupTest(t)
		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			return api.RetryOptions{
				Retryable: true,
				Actions:   []api.RetryAction{{Value: "standard", Label: "Standard retry"}},
			}, nil
		}
		setup.mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
			require.Equal(t, "standard", cfg.Action)
			return api.RequestRetryResult{Status: "retry_requested", RunID: "run-123", TaskID: "task-123"}, nil
		}

		result, err := setup.service.Retry(cli.RetryConfig{
			Target: api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
		})

		require.NoError(t, err)
		require.Equal(t, "task-123", result.TaskID)
		require.Empty(t, setup.mockStdout.String())
	})

	t.Run("uses the only tool cache without prompting", func(t *testing.T) {
		setup := setupTest(t)
		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			return api.RetryOptions{
				Retryable: true,
				Actions:   []api.RetryAction{{Value: "no-tool-cache", Label: "Retry without tool caches"}},
				ToolCaches: []api.RetryToolCache{{
					Name:             "bundler",
					UsageDescription: "Used by test",
				}},
			}, nil
		}
		setup.mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
			require.Equal(t, "no-tool-cache", cfg.Action)
			require.Equal(t, []string{"bundler"}, cfg.ToolCacheNames)
			return api.RequestRetryResult{Status: "retry_requested", RunID: "run-123", TaskID: "task-123"}, nil
		}

		result, err := setup.service.Retry(cli.RetryConfig{
			Target: api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
		})

		require.NoError(t, err)
		require.Equal(t, "task-123", result.TaskID)
		require.Empty(t, setup.mockStdout.String())
	})

	t.Run("does not prompt for debugging", func(t *testing.T) {
		setup := setupTestWithTTY(t)
		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			return api.RetryOptions{
				Retryable: true,
				Actions:   []api.RetryAction{{Value: "standard", Label: "Standard retry"}},
				Debug: api.RetryDebugOptions{
					Supported:        true,
					Placements:       []string{"end", "start"},
					DefaultPlacement: "end",
				},
			}, nil
		}
		setup.mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
			require.Nil(t, cfg.Debug)
			require.Empty(t, cfg.DebugPlacement)
			return api.RequestRetryResult{Status: "retry_requested", RunID: "run-123", TaskID: "task-123"}, nil
		}

		result, err := setup.service.Retry(cli.RetryConfig{
			Target: api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
		})

		require.NoError(t, err)
		require.Equal(t, "task-123", result.TaskID)
		require.NotContains(t, setup.mockStdout.String(), "Open a breakpoint?")
	})

	t.Run("prints available actions when a non-TTY must make a selection", func(t *testing.T) {
		setup := setupTest(t)
		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			require.Equal(t, api.RetryTarget{ID: "target-123", Type: api.RetryTargetInferred}, target)
			return retryOptions(), nil
		}
		setup.mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
			t.Fatal("the API must not request a retry before a action is selected")
			return api.RequestRetryResult{}, nil
		}

		result, err := setup.service.Retry(cli.RetryConfig{
			Target: api.RetryTarget{ID: "target-123", Type: api.RetryTargetInferred},
		})

		require.Nil(t, result)
		require.ErrorIs(t, err, internalErrors.ErrBadRequest)
		require.EqualError(t, err, `retry action selection is required

Available retry actions:
  standard - Standard retry
    The task will run again.
  no-tool-cache - Retry without tool caches
    Only affected tasks will run again.

Choose an action and retry:
  rwx retry --action standard target-123`)
	})

	t.Run("uses flag selections without prompting", func(t *testing.T) {
		setup := setupTest(t)
		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			return retryOptions(), nil
		}
		setup.mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
			require.Equal(t, api.RequestRetryConfig{
				Target: api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
				Action: "standard",
			}, cfg)
			return api.RequestRetryResult{Status: "retry_requested", RunID: "run-123", TaskID: "task-123"}, nil
		}

		result, err := setup.service.Retry(cli.RetryConfig{
			Target: api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
			Action: "standard",
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
			Action: "no-tool-cache",
		})

		require.Nil(t, result)
		require.ErrorIs(t, err, internalErrors.ErrBadRequest)
		require.EqualError(t, err, `tool cache selection is required for retry action "no-tool-cache"

Available tool caches:
  bundler
    Used by test
  golang
    Used by 2 tasks

Choose one or more tool caches and retry:
  rwx runs retry --action no-tool-cache --without-tool-cache bundler run-123`)
	})

	t.Run("prompts for retry actions and tool caches in a TTY", func(t *testing.T) {
		setup := setupTestWithTTY(t)
		_, err := setup.mockStdin.WriteString("2\n1,2\n")
		require.NoError(t, err)

		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			return retryOptions(), nil
		}
		setup.mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
			require.Equal(t, "no-tool-cache", cfg.Action)
			require.Equal(t, []string{"bundler", "golang"}, cfg.ToolCacheNames)
			require.Nil(t, cfg.Debug)
			require.Empty(t, cfg.DebugPlacement)
			return api.RequestRetryResult{Status: "retry_requested", RunID: "run-123", TaskID: "task-123"}, nil
		}

		result, err := setup.service.Retry(cli.RetryConfig{
			Target: api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		output := setup.mockStdout.String()
		require.Contains(t, output, "Select a retry action:")
		require.Contains(t, output, "2. Retry without tool caches (no-tool-cache)")
		require.Contains(t, output, "Select one or more tool caches:")
		require.NotContains(t, output, "Open a breakpoint?")
		require.NotContains(t, output, "Select breakpoint placement:")
	})

	t.Run("enables debugging at the selected placement", func(t *testing.T) {
		setup := setupTest(t)
		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			return retryOptions(), nil
		}
		setup.mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
			require.NotNil(t, cfg.Debug)
			require.True(t, *cfg.Debug)
			require.Equal(t, "end", cfg.DebugPlacement)
			return api.RequestRetryResult{Status: "retry_requested", RunID: "run-123", TaskID: "task-123"}, nil
		}

		_, err := setup.service.Retry(cli.RetryConfig{
			Target:         api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
			Action:         "standard",
			DebugPlacement: "end",
		})

		require.NoError(t, err)
	})

	t.Run("rejects an unavailable debug placement before requesting a retry", func(t *testing.T) {
		setup := setupTest(t)
		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			return retryOptions(), nil
		}
		setup.mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
			t.Fatal("the API must not receive an invalid breakpoint placement")
			return api.RequestRetryResult{}, nil
		}

		result, err := setup.service.Retry(cli.RetryConfig{
			Target:         api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
			Action:         "standard",
			DebugPlacement: "middle",
		})

		require.Nil(t, result)
		require.EqualError(t, err, "breakpoint placement \"middle\" is not available; choose one of: end, start")
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

	t.Run("prints refreshed retry actions after the endpoint rejects an explicit action", func(t *testing.T) {
		setup := setupTest(t)
		initialOptions := retryOptions()
		initialOptions.Actions = append([]api.RetryAction{{Value: "clean", Label: "Clean retry"}}, initialOptions.Actions...)
		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			return initialOptions, nil
		}
		setup.mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
			return api.RequestRetryResult{}, &api.RetryRequestError{
				Message: "This retry configuration is not supported.",
				Errors: []api.RetryFieldError{{
					Field:   "action",
					Message: "`clean` is not available for this task. Choose one of: `standard`, `no-tool-cache`.",
				}},
				Options: retryOptions(),
			}
		}

		result, err := setup.service.Retry(cli.RetryConfig{
			Target: api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
			Action: "clean",
		})

		require.Nil(t, result)
		require.ErrorIs(t, err, internalErrors.ErrBadRequest)
		require.EqualError(t, err, "This retry configuration is not supported.\n\n"+
			"Retry action: `clean` is not available for this task. Choose one of: `standard`, `no-tool-cache`.\n\n"+
			"Available retry actions:\n"+
			"  standard - Standard retry\n"+
			"    The task will run again.\n"+
			"  no-tool-cache - Retry without tool caches\n"+
			"    Only affected tasks will run again.\n\n"+
			"Try:\n"+
			"  rwx tasks retry task-123 --action standard")
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
			Action:           "no-tool-cache",
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
			"  rwx tasks retry task-123 --action no-tool-cache --without-tool-cache bundler")
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
		result, err := setup.service.Retry(cli.RetryConfig{
			Target:         api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
			Action:         "standard",
			DebugPlacement: "middle",
		})

		require.Nil(t, result)
		require.EqualError(t, err, "This retry configuration is not supported.\n\n"+
			"Breakpoint placement: `middle` is not a valid breakpoint placement.\n\n"+
			"Available breakpoint placements:\n"+
			"  end (default)\n"+
			"  start\n\n"+
			"Try:\n"+
			"  rwx tasks retry task-123 --action standard --debug end")
	})

	t.Run("uses a sole refreshed endpoint option for a bare TTY command", func(t *testing.T) {
		setup := setupTestWithTTY(t)

		initialOptions := api.RetryOptions{
			Retryable: true,
			Actions:   []api.RetryAction{{Value: "clean", Label: "Clean retry"}},
		}
		refreshedOptions := api.RetryOptions{
			Retryable: true,
			Actions:   []api.RetryAction{{Value: "standard", Label: "Standard retry"}},
		}
		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			return initialOptions, nil
		}
		calls := 0
		setup.mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
			calls++
			if calls == 1 {
				require.Equal(t, "clean", cfg.Action)
				return api.RequestRetryResult{}, &api.RetryRequestError{
					Message: "This retry configuration is not supported.",
					Errors:  []api.RetryFieldError{{Field: "action", Message: "`clean` is no longer available."}},
					Options: refreshedOptions,
				}
			}

			require.Equal(t, "standard", cfg.Action)
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
		require.NotContains(t, output, "Select a retry action:")
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
			Action: "standard",
		})

		require.Nil(t, result)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Retry: The retry target changed.")
		require.NotContains(t, err.Error(), "target:")
		require.Contains(t, err.Error(), "Available retry actions:")
		require.Contains(t, err.Error(), "Breakpoint:\n  Supported\n  Placements: end (default), start")
		require.Contains(t, err.Error(), "Available tool caches:")
		require.Contains(t, err.Error(), "Try:\n  rwx tasks retry task-123 --action standard")
	})

	t.Run("does not replace an explicit TTY selection", func(t *testing.T) {
		setup := setupTestWithTTY(t)
		initialOptions := api.RetryOptions{
			Retryable: true,
			Actions:   []api.RetryAction{{Value: "clean", Label: "Clean retry"}},
		}
		refreshedOptions := api.RetryOptions{
			Retryable: true,
			Actions:   []api.RetryAction{{Value: "standard", Label: "Standard retry"}},
		}
		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			return initialOptions, nil
		}
		calls := 0
		setup.mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
			calls++
			return api.RequestRetryResult{}, &api.RetryRequestError{
				Message: "This retry configuration is not supported.",
				Errors:  []api.RetryFieldError{{Field: "action", Message: "`clean` is no longer available."}},
				Options: refreshedOptions,
			}
		}

		result, err := setup.service.Retry(cli.RetryConfig{
			Target: api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
			Action: "clean",
		})

		require.Nil(t, result)
		require.Error(t, err)
		require.Equal(t, 1, calls)
		require.Empty(t, setup.mockStdout.String())
		require.Contains(t, err.Error(), "Available retry actions:")
	})

	t.Run("stops after one retry with refreshed TTY options", func(t *testing.T) {
		setup := setupTestWithTTY(t)
		_, err := setup.mockStdin.WriteString("1\n1\n")
		require.NoError(t, err)

		initialOptions := api.RetryOptions{
			Retryable: true,
			Actions:   []api.RetryAction{{Value: "clean", Label: "Clean retry"}},
		}
		refreshedOptions := api.RetryOptions{
			Retryable: true,
			Actions:   []api.RetryAction{{Value: "standard", Label: "Standard retry"}},
		}
		setup.mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
			return initialOptions, nil
		}
		calls := 0
		setup.mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
			calls++
			return api.RequestRetryResult{}, &api.RetryRequestError{
				Message: "This retry configuration is not supported.",
				Errors:  []api.RetryFieldError{{Field: "action", Message: "The selected action is no longer available."}},
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
		result, err := setup.service.Retry(cli.RetryConfig{
			Target:         api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask},
			Action:         "standard",
			DebugPlacement: "end",
		})

		require.Nil(t, result)
		require.EqualError(t, err, "This retry configuration is not supported.\n\n"+
			"Breakpoint: A breakpoint cannot be opened because vault access is required.")
	})
}
