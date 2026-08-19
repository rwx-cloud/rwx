package main

import (
	"encoding/json"
	"fmt"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/spf13/cobra"
)

var retryCmd = newRetryCommand(
	"retry [flags] <run-or-task-id>",
	"Retry a run or task",
	api.RetryTargetInferred,
	"execution",
)

var runsRetryCmd = newRetryCommand(
	"retry [flags] <run-id>",
	"Retry a run",
	api.RetryTargetRun,
	"",
)

var tasksRetryCmd = newRetryCommand(
	"retry [flags] <task-id>",
	"Retry a task",
	api.RetryTargetTask,
	"",
)

func newRetryCommand(use, short string, targetType api.RetryTargetType, groupID string) *cobra.Command {
	var kind string
	var withoutToolCache []string
	var debug bool
	var debugPlacement string

	cmd := &cobra.Command{
		GroupID: groupID,
		Use:     use,
		Short:   short,
		Args:    cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireAccessToken()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var debugSelection *bool
			if cmd.Flags().Changed("debug") {
				debugSelection = &debug
			}

			result, err := service.Retry(cli.RetryConfig{
				Target: api.RetryTarget{
					ID:   args[0],
					Type: targetType,
				},
				Kind:             kind,
				WithoutToolCache: withoutToolCache,
				Debug:            debugSelection,
				DebugPlacement:   debugPlacement,
				OutputJSON:       useJsonOutput(),
			})
			if err != nil {
				return err
			}

			if useJsonOutput() {
				encoded, err := json.Marshal(result)
				if err != nil {
					return err
				}
				fmt.Fprintln(service.Stdout, string(encoded))
				return nil
			}

			if result.TaskID != "" {
				fmt.Fprintf(service.Stdout, "Retry requested for task %s\n", result.TaskID)
			} else {
				fmt.Fprintf(service.Stdout, "Retry requested for run %s\n", result.RunID)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&kind, "kind", "", "retry kind to use")
	cmd.Flags().StringArrayVar(&withoutToolCache, "without-tool-cache", nil, "tool cache to exclude; can be specified multiple times")
	cmd.Flags().BoolVar(&debug, "debug", false, "open a breakpoint during the retried task")
	cmd.Flags().StringVar(&debugPlacement, "debug-placement", "", "where to open the breakpoint: start or end")
	return cmd
}
