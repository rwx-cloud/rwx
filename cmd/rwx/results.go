package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/rwx-cloud/rwx/internal/git"

	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
)

var (
	ResultsWait       bool
	ResultsFailFast   bool
	ResultsOpen       bool
	ResultsBranch     string
	ResultsRepo       string
	ResultsDefinition string
	ResultsCommit     string
	ResultsTaskKey    string

	resultsCmd = &cobra.Command{
		GroupID: "outputs",
		Use:     "results [run-id | run-id --task <key>]",
		Short:   "Get results for a run",
		Args:    cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireAccessToken()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			useJson := useJsonOutput()
			taskKeySet := cmd.Flags().Changed("task")

			var runID string
			runIDFromGit := false
			if len(args) > 0 {
				runID = args[0]
			} else {
				var err error
				runID, err = service.ResolveRunIDFromGitContext(cli.ResolveRunIDConfig{
					BranchName:     ResultsBranch,
					RepositoryName: ResultsRepo,
					DefinitionPath: ResultsDefinition,
					CommitSha:      ResultsCommit,
				})
				if err != nil {
					return HandleAmbiguousDefinitionPathError(err, ResultsBranch, ResultsRepo)
				}
				runIDFromGit = true
			}

			result, err := service.GetRunStatus(cli.GetRunStatusConfig{
				RunID:    runID,
				TaskKey:  ResultsTaskKey,
				Wait:     ResultsWait,
				FailFast: ResultsFailFast,
				Json:     useJson,
			})
			if err != nil {
				if taskKeySet {
					return handleResultsTaskKeyError(err)
				}
				return err
			}

			var promptResult *cli.GetRunPromptResult
			var promptErr error
			if taskKeySet {
				promptResult, promptErr = service.GetRunPromptByTaskKey(runID, ResultsTaskKey)
				if promptErr != nil {
					promptErr = handleResultsTaskKeyError(promptErr)
				}
			} else {
				promptID := result.RunID
				if result.TaskID != "" {
					promptID = result.TaskID
				}
				promptResult, promptErr = service.GetRunPrompt(promptID)
			}

			if useJson {
				jsonOutput := struct {
					RunID        string
					TaskID       string `json:",omitempty"`
					ResultStatus string
					Completed    bool
					Prompt       string `json:",omitempty"`
				}{
					RunID:        result.RunID,
					TaskID:       result.TaskID,
					ResultStatus: result.ResultStatus,
					Completed:    result.Completed,
				}
				if promptErr == nil {
					jsonOutput.Prompt = promptResult.Prompt
				}
				resultJson, err := json.Marshal(jsonOutput)
				if err != nil {
					return err
				}
				fmt.Println(string(resultJson))
			} else {
				if runIDFromGit && ResultsBranch == "" && ResultsRepo == "" && result.Commit != "" {
					if head := service.GitClient.GetHead(); head != "" {
						if note := git.CommitMismatchNote(head, result.Commit); note != "" {
							fmt.Println(note)
						}
					}
				}
				if result.TaskURL != "" {
					fmt.Printf("Task URL: %s\n", result.TaskURL)
				} else if result.RunURL != "" {
					fmt.Printf("Run URL: %s\n", result.RunURL)
				}
				statusLabel := "Run"
				if result.TaskURL != "" {
					statusLabel = "Task"
				}
				if result.Completed {
					fmt.Printf("%s result status: %s\n", statusLabel, result.ResultStatus)
				} else {
					fmt.Printf("%s status: %s (in progress)\n", statusLabel, result.ResultStatus)
				}

				if promptErr == nil {
					fmt.Printf("\n%s", promptResult.Prompt)
				}
			}

			if ResultsOpen {
				openURL := result.RunURL
				if result.TaskURL != "" {
					openURL = result.TaskURL
				}
				if openURL != "" {
					if err := open.Run(openURL); err != nil {
						fmt.Fprintf(os.Stderr, "Failed to open browser.\n")
					}
				}
			}

			if result.Completed && result.ResultStatus != "succeeded" {
				return HandledError
			}

			return nil
		},
	}
)

func init() {
	resultsCmd.Flags().BoolVar(&ResultsOpen, "open", false, "open the run in a browser")
	resultsCmd.Flags().BoolVar(&ResultsWait, "wait", false, "poll for the run to complete and report the result status")
	resultsCmd.Flags().BoolVar(&ResultsFailFast, "fail-fast", false, "stop waiting when failures are available (only has an effect when used with --wait)")
	resultsCmd.Flags().StringVar(&ResultsBranch, "branch", "", "get results for a specific branch instead of the current git branch")
	resultsCmd.Flags().StringVar(&ResultsRepo, "repo", "", "get results for a specific repository instead of the current git repository")
	resultsCmd.Flags().StringVar(&ResultsDefinition, "definition", "", "get results for a specific definition path")
	resultsCmd.Flags().StringVar(&ResultsCommit, "commit", "", "get results for a specific commit SHA")
	resultsCmd.Flags().StringVar(&ResultsTaskKey, "task", "", "task key (e.g., ci.checks.lint); resolves the task by key instead of ID")
}

func handleResultsTaskKeyError(err error) error {
	var ambiguousErr *api.AmbiguousTaskKeyError
	if errors.As(err, &ambiguousErr) {
		return errors.WrapSentinel(errors.New(ambiguousErr.Error()), errors.ErrAmbiguousTaskKey)
	}

	return err
}

func HandleAmbiguousDefinitionPathError(err error, branch, repo string) error {
	var ambiguousErr *api.AmbiguousDefinitionPathError
	if errors.As(err, &ambiguousErr) {
		msg := ambiguousErr.Error()
		for _, path := range ambiguousErr.MatchingDefinitionPaths {
			cmd := "rwx results"
			if branch != "" {
				cmd += " --branch " + branch
			}
			if repo != "" {
				cmd += " --repo " + repo
			}
			cmd += " --definition " + path
			msg += "\n  " + cmd
		}
		return errors.WrapSentinel(errors.New(msg), errors.ErrAmbiguousDefinitionPath)
	}

	return err
}
