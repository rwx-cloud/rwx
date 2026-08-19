package main

import (
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/mocks"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestRetryCommandsAreRegistered(t *testing.T) {
	tests := []struct {
		name       string
		parent     *cobra.Command
		command    string
		identifier string
	}{
		{name: "inferred target", parent: rootCmd, command: "retry", identifier: "run-or-task-id"},
		{name: "run", parent: runsCmd, command: "retry", identifier: "run-id"},
		{name: "task", parent: tasksCmd, command: "retry", identifier: "task-id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := findSubcommand(tt.parent, tt.command)
			require.NotNil(t, cmd)
			require.Contains(t, cmd.Use, tt.identifier)
			require.NoError(t, cmd.Args(cmd, []string{"target-123"}))
			require.Error(t, cmd.Args(cmd, nil))
			require.Error(t, cmd.Args(cmd, []string{"one", "two"}))

			for _, flag := range []string{"kind", "without-tool-cache", "debug", "debug-placement"} {
				require.NotNil(t, cmd.Flags().Lookup(flag), "retry should expose --%s", flag)
			}
		})
	}

	tasks := findSubcommand(rootCmd, "tasks")
	require.NotNil(t, tasks)
	require.False(t, tasks.Runnable())
	require.NotNil(t, findSubcommand(tasks, "retry"))
}

func TestRetryCommandPassesFlagSelectionsToService(t *testing.T) {
	originalService := service
	originalFormat := Format
	t.Cleanup(func() {
		service = originalService
		Format = originalFormat
	})

	mockAPI := new(mocks.API)
	mockAPI.MockGetRetryOptions = func(target api.RetryTarget) (api.RetryOptions, error) {
		require.Equal(t, api.RetryTarget{ID: "task-123", Type: api.RetryTargetTask}, target)
		return api.RetryOptions{
			Retryable: true,
			Kinds: []api.RetryKind{{
				Value: "no-tool-cache",
				Label: "Retry without tool caches",
			}},
			ToolCaches: []api.RetryToolCache{{Name: "bundler"}, {Name: "golang"}},
		}, nil
	}
	mockAPI.MockRequestRetry = func(cfg api.RequestRetryConfig) (api.RequestRetryResult, error) {
		require.Equal(t, "no-tool-cache", cfg.Kind)
		require.Equal(t, []string{"bundler", "golang"}, cfg.ToolCacheNames)
		require.NotNil(t, cfg.Debug)
		require.False(t, *cfg.Debug)
		return api.RequestRetryResult{
			Status: "retry_requested",
			RunID:  "run-123",
			TaskID: "task-123",
		}, nil
	}

	var stdout strings.Builder
	service = cli.Service{Config: cli.Config{
		APIClient: mockAPI,
		Stdout:    &stdout,
	}}
	cmd := newRetryCommand("retry [flags] <task-id>", "Retry a task", api.RetryTargetTask, "")
	require.NoError(t, cmd.Flags().Parse([]string{
		"--kind", "no-tool-cache",
		"--without-tool-cache", "bundler",
		"--without-tool-cache", "golang",
		"--debug=false",
	}))

	err := cmd.RunE(cmd, []string{"task-123"})

	require.NoError(t, err)
	require.Equal(t, "Retry requested for task task-123\n", stdout.String())
}
