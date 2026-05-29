package main

import (
	"path/filepath"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/errors"

	"github.com/spf13/cobra"
)

var (
	LogsOutputDirectory string
	LogsAutoExtract     bool
	LogsZip             bool
	LogsOpen            bool
	LogsTaskKey         string

	logsCmd = &cobra.Command{
		GroupID: "outputs",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireAccessToken()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			taskKeySet := cmd.Flags().Changed("task")

			if taskKeySet {
				if len(args) > 1 {
					return errors.New("accepts at most 1 arg (run-id) when --task is used")
				}
			} else {
				if len(args) == 0 {
					return errors.New("a task ID or --task flag is required")
				}
				if len(args) > 1 {
					return errors.New("accepts at most 1 arg (task-id)")
				}
			}

			outputDirectorySet := cmd.Flags().Changed("output-directory")
			outputDirAliasSet := cmd.Flags().Changed("output-dir")
			if outputDirectorySet && outputDirAliasSet {
				return errors.New("output-directory and output-dir cannot be used together")
			}

			var err error

			outputDirectoryExplicitlySet := outputDirectorySet || outputDirAliasSet
			outputDirectory := LogsOutputDirectory
			if !outputDirectoryExplicitlySet {
				outputDirectory, err = cli.FindDefaultDownloadsDir()
				if err != nil {
					return errors.Wrap(err, "unable to determine default logs directory")
				}
			}
			absOutputDirectory, err := filepath.Abs(outputDirectory)
			if err != nil {
				return errors.Wrapf(err, "unable to resolve absolute path for %s", outputDirectory)
			}

			useJson := useJsonOutput()

			cfg := cli.DownloadLogsConfig{
				OutputDir:              absOutputDirectory,
				OutputDirExplicitlySet: outputDirectoryExplicitlySet,
				Json:                   useJson,
				Zip:                    LogsZip,
				Open:                   LogsOpen,
			}

			if taskKeySet {
				var runID string
				if len(args) > 0 {
					runID = args[0]
				} else {
					runID, err = service.ResolveRunIDFromGitContext()
					if err != nil {
						return err
					}
				}
				cfg.RunID = runID
				cfg.TaskKey = LogsTaskKey

				_, err = service.DownloadLogs(cfg)
				if err != nil {
					return handleTaskKeyError(err)
				}
				return nil
			}

			cfg.TaskID = args[0]
			_, err = service.DownloadLogs(cfg)
			return err
		},
		Short: "Download logs for a task",
		Use:   "logs [task-id | run-id --task <key>] [flags]",
	}
)

func init() {
	logsCmd.Flags().StringVar(&LogsOutputDirectory, "output-directory", "", "output directory for downloaded logs (defaults to .rwx/downloads folder)")
	logsCmd.Flags().StringVar(&LogsOutputDirectory, "output-dir", "", "output directory for downloaded logs (defaults to .rwx/downloads folder)")
	if err := logsCmd.Flags().MarkHidden("output-dir"); err != nil {
		panic(err)
	}
	logsCmd.Flags().BoolVar(&LogsAutoExtract, "auto-extract", false, "automatically extract zip archives")
	if err := logsCmd.Flags().MarkHidden("auto-extract"); err != nil {
		panic(err)
	}
	logsCmd.Flags().BoolVar(&LogsZip, "zip", false, "skip extraction and save raw zip archive")
	logsCmd.Flags().BoolVar(&LogsOpen, "open", false, "automatically open the downloaded file(s)")
	logsCmd.Flags().StringVar(&LogsTaskKey, "task", "", "task key (e.g., ci.checks.lint); resolves the task by key instead of ID")
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
