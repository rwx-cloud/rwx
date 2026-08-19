package main

import (
	"encoding/json"
	"fmt"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	internalErrors "github.com/rwx-cloud/rwx/internal/errors"
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
	var action string
	var withoutToolCache []string
	var debug string

	cmd := &cobra.Command{
		GroupID: groupID,
		Use:     use,
		Short:   short,
		Args:    cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireAccessToken()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := service.Retry(cli.RetryConfig{
				Target: api.RetryTarget{
					ID:   args[0],
					Type: targetType,
				},
				Action:           action,
				WithoutToolCache: withoutToolCache,
				DebugPlacement:   debug,
				OutputJSON:       useJsonOutput(),
			})
			if err != nil {
				var requestErr *api.RetryRequestError
				if useJsonOutput() && internalErrors.As(err, &requestErr) {
					encoded, encodeErr := json.Marshal(requestErr)
					if encodeErr != nil {
						return encodeErr
					}
					fmt.Fprintln(service.Stdout, string(encoded))
					return internalErrors.WrapSentinel(requestErr, HandledError)
				}
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

	cmd.Flags().StringVar(&action, "action", "", "retry action to use")
	cmd.Flags().StringArrayVar(&withoutToolCache, "without-tool-cache", nil, "tool cache to exclude; can be specified multiple times")
	cmd.Flags().StringVar(&debug, "debug", "", "open a breakpoint at the start or end of the retried task")
	return cmd
}
