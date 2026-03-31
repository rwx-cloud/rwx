package main

import (
	"encoding/json"
	"fmt"

	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/git"

	"github.com/spf13/cobra"
)

var (
	ResultsWait       bool
	ResultsFailFast   bool
	ResultsBranch     string
	ResultsRepo       string
	ResultsDefinition string

	resultsCmd = &cobra.Command{
		GroupID: "outputs",
		Use:     "results [run-id]",
		Short:   "Get results for a run",
		Args:    cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireAccessToken()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			useJson := useJsonOutput()

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
				})
				if err != nil {
					return err
				}
				runIDFromGit = true
			}

			result, err := service.GetRunStatus(cli.GetRunStatusConfig{
				RunID:    runID,
				Wait:     ResultsWait,
				FailFast: ResultsFailFast,
				Json:     useJson,
			})
			if err != nil {
				return err
			}

			if useJson {
				jsonOutput := struct {
					RunID        string
					ResultStatus string
					Completed    bool
				}{
					RunID:        result.RunID,
					ResultStatus: result.ResultStatus,
					Completed:    result.Completed,
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
				if result.RunURL != "" {
					fmt.Printf("Run URL: %s\n", result.RunURL)
				}
				if result.Completed {
					fmt.Printf("Run result status: %s\n", result.ResultStatus)
				} else {
					fmt.Printf("Run status: %s (in progress)\n", result.ResultStatus)
				}

				promptResult, err := service.GetRunPrompt(result.RunID)
				if err == nil {
					fmt.Printf("\n%s", promptResult.Prompt)
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
	resultsCmd.Flags().BoolVar(&ResultsWait, "wait", false, "poll for the run to complete and report the result status")
	resultsCmd.Flags().BoolVar(&ResultsFailFast, "fail-fast", false, "stop waiting when failures are available (only has an effect when used with --wait)")
	resultsCmd.Flags().StringVar(&ResultsBranch, "branch", "", "get results for a specific branch instead of the current git branch")
	resultsCmd.Flags().StringVar(&ResultsRepo, "repo", "", "get results for a specific repository instead of the current git repository")
	resultsCmd.Flags().StringVar(&ResultsDefinition, "definition", "", "get results for a specific definition path")
}
