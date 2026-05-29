package artifacts

import (
	"fmt"
	"path/filepath"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/errors"

	"github.com/spf13/cobra"
)

var (
	downloadOutputDir   string
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

			outputDirSet := cmd.Flags().Changed("output-dir")

			if taskKeySet {
				return runDownloadWithTaskKey(svc, args, outputDirSet, useJsonOutput())
			}

			taskID := args[0]

			if downloadAll {
				absOutputDir, err := resolveDownloadOutputDir(outputDirSet)
				if err != nil {
					return err
				}

				useJson := useJsonOutput()
				_, err = svc.DownloadAllArtifacts(cli.DownloadAllArtifactsConfig{
					TaskID:                 taskID,
					OutputDir:              absOutputDir,
					OutputDirExplicitlySet: outputDirSet,
					Json:                   useJson,
					AutoExtract:            downloadAutoExtract,
					Open:                   downloadOpen,
				})
				return err
			}

			artifactKey := args[1]

			absOutputDir, err := resolveDownloadOutputDir(outputDirSet)
			if err != nil {
				return err
			}

			useJson := useJsonOutput()
			_, err = svc.DownloadArtifact(cli.DownloadArtifactConfig{
				TaskID:                 taskID,
				ArtifactKey:            artifactKey,
				OutputDir:              absOutputDir,
				OutputDirExplicitlySet: outputDirSet,
				Json:                   useJson,
				AutoExtract:            downloadAutoExtract,
				Open:                   downloadOpen,
			})
			return err
		},
		Short: "Download an artifact from a task",
		Use:   "download [task-id | run-id --task <key>] [artifact-key] [flags]",
	}

	DownloadCmd.Flags().StringVar(&downloadOutputDir, "output-dir", "", "output directory for downloaded artifacts (defaults to .rwx/downloads folder)")
	DownloadCmd.Flags().BoolVar(&downloadAutoExtract, "auto-extract", false, "automatically extract directory tar archives")
	DownloadCmd.Flags().BoolVar(&downloadOpen, "open", false, "automatically open the downloaded file(s)")
	DownloadCmd.Flags().BoolVar(&downloadAll, "all", false, "download all artifacts for the task")
	DownloadCmd.Flags().StringVar(&downloadTaskKey, "task", "", "task key (e.g., ci.checks.lint); resolves the task by key instead of ID")
}

func runDownloadWithTaskKey(svc cli.Service, args []string, outputDirSet bool, useJson bool) error {
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

		absOutputDir, err := resolveDownloadOutputDir(outputDirSet)
		if err != nil {
			return err
		}

		_, err = svc.DownloadAllArtifacts(cli.DownloadAllArtifactsConfig{
			RunID:                  runID,
			TaskKey:                downloadTaskKey,
			OutputDir:              absOutputDir,
			OutputDirExplicitlySet: outputDirSet,
			Json:                   useJson,
			AutoExtract:            downloadAutoExtract,
			Open:                   downloadOpen,
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

	absOutputDir, err := resolveDownloadOutputDir(outputDirSet)
	if err != nil {
		return err
	}

	_, err = svc.DownloadArtifact(cli.DownloadArtifactConfig{
		RunID:                  runID,
		TaskKey:                downloadTaskKey,
		ArtifactKey:            artifactKey,
		OutputDir:              absOutputDir,
		OutputDirExplicitlySet: outputDirSet,
		Json:                   useJson,
		AutoExtract:            downloadAutoExtract,
		Open:                   downloadOpen,
	})
	if err != nil {
		return handleTaskKeyError(err)
	}
	return nil
}

func resolveDownloadOutputDir(explicitlySet bool) (string, error) {
	outputDir := downloadOutputDir
	if !explicitlySet {
		var err error
		outputDir, err = cli.FindDefaultDownloadsDir()
		if err != nil {
			return "", errors.Wrap(err, "unable to determine default downloads directory")
		}
	}

	absOutputDir, err := filepath.Abs(outputDir)
	if err != nil {
		return "", errors.Wrapf(err, "unable to resolve absolute path for %s", outputDir)
	}
	return absOutputDir, nil
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
