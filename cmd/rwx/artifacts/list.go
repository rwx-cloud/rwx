package artifacts

import (
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/errors"

	"github.com/spf13/cobra"
)

var (
	listTaskKey string

	ListCmd *cobra.Command
)

func InitList(getService func() cli.Service, useJsonOutput func() bool) {
	ListCmd = &cobra.Command{
		Args: func(cmd *cobra.Command, args []string) error {
			taskKeySet := cmd.Flags().Changed("task")
			if taskKeySet {
				if len(args) > 1 {
					return errors.Errorf("accepts at most 1 arg (run-id) when --task is used, received %d", len(args))
				}
			} else {
				if len(args) != 1 {
					return errors.Errorf("accepts 1 arg (task-id), received %d", len(args))
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			taskKeySet := cmd.Flags().Changed("task")
			useJson := useJsonOutput()
			svc := getService()

			cfg := cli.ListArtifactsConfig{
				Json: useJson,
			}

			if taskKeySet {
				var runID string
				var err error
				if len(args) > 0 {
					runID = args[0]
				} else {
					runID, err = svc.ResolveRunIDFromGitContext()
					if err != nil {
						return err
					}
				}
				cfg.RunID = runID
				cfg.TaskKey = listTaskKey

				_, err = svc.ListArtifacts(cfg)
				if err != nil {
					return handleTaskKeyError(err)
				}
				return nil
			}

			cfg.TaskID = args[0]
			_, err := svc.ListArtifacts(cfg)
			return err
		},
		Short: "List artifacts for a task",
		Use:   "list [task-id | run-id --task <key>] [flags]",
	}

	ListCmd.Flags().StringVar(&listTaskKey, "task", "", "task key (e.g., ci.checks.lint); resolves the task by key instead of ID")
}
