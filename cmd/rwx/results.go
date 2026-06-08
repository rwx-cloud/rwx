package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

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
		Long: `Get results for a run.

The default output is an LLM-friendly prompt for investigating run failures: the
run's status plus a failure summary you can hand to a coding agent.

Pass --output json to get structured run and task fields instead. For the full
list of JSON fields, see https://rwx.com/docs/results or run:

    rwx docs pull /results`,
		Args: cobra.MaximumNArgs(1),
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

				baseJson, err := json.Marshal(jsonOutput)
				if err != nil {
					return err
				}
				var base map[string]any
				if err := json.Unmarshal(baseJson, &base); err != nil {
					return err
				}

				// Fetch the enriched payload and fold it into the base output. An
				// empty object means enrichment is not enabled for the org, so the
				// merge is a no-op. Pass the original run/task identifier (not the
				// resolved run ID) so a task-scoped lookup stays task-scoped.
				details, err := service.GetRunDetails(cli.GetRunDetailsConfig{
					RunID:   runID,
					TaskKey: ResultsTaskKey,
				})
				if err != nil {
					return err
				}
				if len(details) > 0 {
					base = MergeEnrichedResults(base, details)
				}

				resultJson, err := json.Marshal(base)
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

// MergeEnrichedResults overlays the enriched /details payload onto the base JSON
// map. The enriched payload uses snake_case keys, which are rewritten to
// PascalCase to match the casing of the base payload. The enriched id is kept and
// surfaced as ID (mirroring the run/task id already exposed as RunID/TaskID), so
// callers can read it under any of those keys. Existing base keys are preserved on
// collision.
func MergeEnrichedResults(base, details map[string]any) map[string]any {
	merged := make(map[string]any, len(base)+len(details))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range pascalCaseKeys(details).(map[string]any) {
		if _, exists := merged[key]; !exists {
			merged[key] = value
		}
	}

	return merged
}

// pascalCaseKeys recursively rewrites map keys from snake_case to PascalCase,
// recursing into nested maps and slices. Non-map/slice values are returned
// unchanged.
func pascalCaseKeys(v any) any {
	switch value := v.(type) {
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, nested := range value {
			result[pascalCase(key)] = pascalCaseKeys(nested)
		}
		return result
	case []any:
		for i, element := range value {
			value[i] = pascalCaseKeys(element)
		}
		return value
	default:
		return v
	}
}

// pascalCase converts a snake_case identifier to PascalCase by upper-casing the
// first rune of each underscore-delimited segment (e.g. completed_runtime_seconds
// -> CompletedRuntimeSeconds). The "id" segment is treated as an initialism and
// rendered as "ID" to match the base payload's RunID/TaskID keys.
func pascalCase(key string) string {
	var builder strings.Builder
	for _, segment := range strings.Split(key, "_") {
		if segment == "" {
			continue
		}
		if strings.EqualFold(segment, "id") {
			builder.WriteString("ID")
			continue
		}
		runes := []rune(segment)
		builder.WriteString(strings.ToUpper(string(runes[0])))
		builder.WriteString(string(runes[1:]))
	}
	return builder.String()
}
