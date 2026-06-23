package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/text"

	"github.com/spf13/cobra"
)

var (
	RunsListRepositories      []string
	RunsListBranches          []string
	RunsListCommits           []string
	RunsListDefinitions       []string
	RunsListResultStatuses    []string
	RunsListExecutionStatuses []string
	RunsListMine              bool
	RunsListLimit             int
	RunsListCursor            string

	runsListCmd = &cobra.Command{
		Use:     "list [flags]",
		Aliases: []string{"ls"},
		Short:   "List runs",
		Long: `List runs, most recent first.

The default output is a table. Pass --json (or --format json) for the structured
payload, which includes a NextCursor for deliberate paging.

Results are paginated: the default page shows the most recent runs and --limit
sets the page size (capped at 100). When more runs remain, pass the reported
cursor to --cursor to fetch the next page.

For a given run's full payload, run 'rwx results <id> --json'.`,
		Args: cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireAccessToken()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			useJson := useJsonOutput()

			result, err := service.ListRuns(cli.ListRunsConfig{
				RepositoryNames:   RunsListRepositories,
				Branches:          RunsListBranches,
				CommitShas:        RunsListCommits,
				DefinitionPaths:   RunsListDefinitions,
				ResultStatuses:    RunsListResultStatuses,
				ExecutionStatuses: RunsListExecutionStatuses,
				MyRuns:            RunsListMine,
				Limit:             RunsListLimit,
				Cursor:            RunsListCursor,
			})
			if err != nil {
				return err
			}

			// Open-ended filter near-misses come back on a successful (200)
			// response, so they are a non-fatal hint on stderr rather than an error.
			printRunFilterSuggestions(os.Stderr, result.Suggestions)

			if useJson {
				return printRunsJSON(result)
			}

			if len(result.Runs) == 0 {
				fmt.Fprintln(os.Stdout, "No runs found.")
				return nil
			}

			renderRunsTable(os.Stdout, result.Runs)
			if result.Pagination.NextCursor != nil {
				fmt.Fprintf(os.Stderr, "\nMore runs available. Fetch the next page with --cursor %s\n", *result.Pagination.NextCursor)
			}

			return nil
		},
	}
)

func init() {
	// All filters are repeatable. StringSliceVar (not StringArrayVar) accepts both
	// repeated flags (--branch a --branch b) and a comma-separated value
	// (--branch a,b); a single value is sent in the scalar query-param form.
	runsListCmd.Flags().StringSliceVar(&RunsListRepositories, "repository", nil, "filter by repository name, case-insensitive (repeatable)")
	runsListCmd.Flags().StringSliceVar(&RunsListBranches, "branch", nil, "filter by branch name, case-insensitive (repeatable)")
	runsListCmd.Flags().StringSliceVar(&RunsListCommits, "commit", nil, "filter by commit SHA (repeatable)")
	runsListCmd.Flags().StringSliceVar(&RunsListDefinitions, "definition", nil, "filter by definition path (repeatable)")
	runsListCmd.Flags().StringSliceVar(&RunsListResultStatuses, "result-status", nil, "filter by result status, repeatable: succeeded, debugged, sandboxed, failed, no_result")
	runsListCmd.Flags().StringSliceVar(&RunsListExecutionStatuses, "execution-status", nil, "filter by execution status, repeatable: waiting, in_progress, finished, aborted")
	runsListCmd.Flags().BoolVar(&RunsListMine, "mine", false, "only list runs you triggered")
	runsListCmd.Flags().IntVar(&RunsListLimit, "limit", 0, "page size (max 100; server default applies when unset)")
	runsListCmd.Flags().StringVar(&RunsListCursor, "cursor", "", "cursor for fetching the next page (from a prior result's NextCursor)")
}

// Title and branch are the most variable-length columns, so they are capped to
// keep rows from overflowing the terminal; the rest are naturally short.
const (
	maxTitleWidth  = 40
	maxBranchWidth = 24
)

func renderRunsTable(w io.Writer, runs []api.RunSummary) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TITLE\tID\tSTATUS\tREPO\tBRANCH\tCOMMIT\tDEFINITION\tCREATED")
	for _, run := range runs {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			text.Truncate(run.Title, maxTitleWidth),
			run.ID,
			runStatusLabel(run.Status),
			run.RepositoryName,
			text.Truncate(run.Branch, maxBranchWidth),
			shortCommitSha(run.CommitSha),
			run.DefinitionPath,
			derefString(run.CreatedAt),
		)
	}
	tw.Flush()
}

// runStatusLabel collapses the nested status into one column: while a run is
// still executing, its execution phase is the meaningful state; once finished,
// the result is.
func runStatusLabel(status api.RunStatus) string {
	if status.Execution != "" && status.Execution != "finished" {
		return status.Execution
	}
	return status.Result
}

func shortCommitSha(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// printRunsJSON emits the typed result with PascalCase keys, matching the casing
// used by 'rwx results --output json' (see MergeEnrichedResults) so the two
// interoperate.
func printRunsJSON(result *api.ListRunsResult) error {
	encoded, err := runsJSON(result)
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

// runsJSON rewrites the snake_case wire keys to PascalCase, matching the casing of
// 'rwx results --output json'.
func runsJSON(result *api.ListRunsResult) ([]byte, error) {
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	var asMap map[string]any
	if err := json.Unmarshal(encoded, &asMap); err != nil {
		return nil, err
	}
	return json.Marshal(pascalCaseKeys(asMap))
}

func printRunFilterSuggestions(w io.Writer, suggestions map[string][]api.RunFilterSuggestion) {
	for kind, entries := range suggestions {
		for _, entry := range entries {
			if len(entry.Suggestions) == 0 {
				continue
			}
			fmt.Fprintf(w, "No exact match for %s %q. Did you mean: %s?\n",
				strings.TrimSuffix(kind, "s"), entry.Value, strings.Join(entry.Suggestions, ", "))
		}
	}
}
