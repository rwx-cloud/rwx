package artifacts

import (
	"fmt"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/errors"

	"github.com/spf13/cobra"
)

var (
	downloadOutput      string
	downloadAutoExtract bool
	downloadOpen        bool
	downloadAll         bool
	downloadTaskKey     string

	DownloadCmd *cobra.Command
)

func InitDownload(requireAccessToken func() error, getService func() cli.Service, useJsonOutput func() bool) {
	DownloadCmd = &cobra.Command{
		Args: func(cmd *cobra.Command, args []string) error {
			taskKeySet := cmd.Flags().Changed("task")

			if taskKeySet {
				// With --task:
				//   --all: 0 or 1 args (optional run-id)
				//   no --all: 1 or 2 args (optional run-id + required artifact-key)
				if downloadAll {
					if len(args) > 1 {
						return fmt.Errorf("accepts at most 1 arg (run-id) when --task and --all are used, received %d", len(args))
					}
				} else {
					if len(args) == 0 || len(args) > 2 {
						return fmt.Errorf("accepts 1-2 args ([run-id] <artifact-key>) when --task is used, received %d", len(args))
					}
				}
			} else {
				// Without --task (existing behavior):
				//   --all: exactly 1 arg (task-id)
				//   no --all: exactly 2 args (task-id, artifact-key)
				if downloadAll {
					if len(args) != 1 {
						return fmt.Errorf("accepts 1 arg(s) when --all is used, received %d", len(args))
					}
				} else {
					if len(args) != 2 {
						return fmt.Errorf("accepts 2 arg(s), received %d", len(args))
					}
				}
			}
			return nil
		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireAccessToken()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			taskKeySet := cmd.Flags().Changed("task")
			svc := getService()

			outputSet := cmd.Flags().Changed("output")

			if taskKeySet {
				return runDownloadWithTaskKey(svc, args, outputSet, useJsonOutput())
			}

			taskID := args[0]

			absOutput, err := cli.ResolveDownloadOutput(downloadOutput, outputSet)
			if err != nil {
				return errors.Wrap(err, "unable to determine artifact output")
			}

			if downloadAll {
				useJson := useJsonOutput()
				_, err = svc.DownloadAllArtifacts(cli.DownloadAllArtifactsConfig{
					TaskID:              taskID,
					Output:              absOutput,
					OutputExplicitlySet: outputSet,
					Json:                useJson,
					AutoExtract:         downloadAutoExtract,
					Open:                downloadOpen,
				})
				return err
			}

			artifactKey := args[1]

			useJson := useJsonOutput()
			_, err = svc.DownloadArtifact(cli.DownloadArtifactConfig{
				TaskID:              taskID,
				ArtifactKey:         artifactKey,
				Output:              absOutput,
				OutputExplicitlySet: outputSet,
				Json:                useJson,
				AutoExtract:         downloadAutoExtract,
				Open:                downloadOpen,
			})
			return err
		},
		Short: "Download an artifact from a task",
		Use:   "download [task-id | run-id --task <key>] [artifact-key] [flags]",
	}

	DownloadCmd.Flags().StringVar(&downloadOutput, "output", "", "output path for the downloaded artifact")
	DownloadCmd.Flags().BoolVar(&downloadAutoExtract, "auto-extract", false, "automatically extract directory tar archives")
	DownloadCmd.Flags().BoolVar(&downloadOpen, "open", false, "automatically open the downloaded file(s)")
	DownloadCmd.Flags().BoolVar(&downloadAll, "all", false, "download all artifacts for the task")
	DownloadCmd.Flags().StringVar(&downloadTaskKey, "task", "", "task key (e.g., ci.checks.lint); resolves the task by key instead of ID")
}

func runDownloadWithTaskKey(svc cli.Service, args []string, outputSet bool, useJson bool) error {
	var runID string
	var artifactKey string
	var err error

	if downloadAll {
		if len(args) > 0 {
			runID = args[0]
		} else {
			runID, err = svc.ResolveRunIDFromGitContext()
			if err != nil {
				return err
			}
		}

		absOutput, err := cli.ResolveDownloadOutput(downloadOutput, outputSet)
		if err != nil {
			return errors.Wrap(err, "unable to determine artifact output")
		}

		_, err = svc.DownloadAllArtifacts(cli.DownloadAllArtifactsConfig{
			RunID:               runID,
			TaskKey:             downloadTaskKey,
			Output:              absOutput,
			OutputExplicitlySet: outputSet,
			Json:                useJson,
			AutoExtract:         downloadAutoExtract,
			Open:                downloadOpen,
		})
		if err != nil {
			return handleTaskKeyError(err)
		}
		return nil
	}

	// Without --all: 1 arg = artifact-key (infer run-id), 2 args = run-id + artifact-key
	if len(args) == 2 {
		runID = args[0]
		artifactKey = args[1]
	} else {
		artifactKey = args[0]
		runID, err = svc.ResolveRunIDFromGitContext()
		if err != nil {
			return err
		}
	}

	absOutput, err := cli.ResolveDownloadOutput(downloadOutput, outputSet)
	if err != nil {
		return errors.Wrap(err, "unable to determine artifact output")
	}

	_, err = svc.DownloadArtifact(cli.DownloadArtifactConfig{
		RunID:               runID,
		TaskKey:             downloadTaskKey,
		ArtifactKey:         artifactKey,
		Output:              absOutput,
		OutputExplicitlySet: outputSet,
		Json:                useJson,
		AutoExtract:         downloadAutoExtract,
		Open:                downloadOpen,
	})
	if err != nil {
		return handleTaskKeyError(err)
	}
	return nil
}

// handleTaskKeyError formats task-key-specific errors for user display.
// Sentinels are preserved so telemetry can classify the error.
func handleTaskKeyError(err error) error {
	var ambiguousErr *api.AmbiguousTaskKeyError
	if errors.As(err, &ambiguousErr) {
		return errors.WrapSentinel(errors.New(ambiguousErr.Error()), errors.ErrAmbiguousTaskKey)
	}

	return err
}
