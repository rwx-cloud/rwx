package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/stretchr/testify/require"
)

func TestRunsListIsRegistered(t *testing.T) {
	listCmd := findSubcommand(runsCmd, "list")
	require.NotNil(t, listCmd, "rwx runs list should exist")
	require.False(t, listCmd.Hidden, "list is a real action and should be visible")
	require.Contains(t, listCmd.Aliases, "ls")

	for _, name := range []string{"repository", "branch", "commit", "definition", "result-status", "execution-status", "mine", "limit", "cursor"} {
		require.NotNil(t, listCmd.Flags().Lookup(name), "list should expose the --%s flag", name)
	}
}

func TestRenderRunsTable(t *testing.T) {
	created := "2026-06-23T12:00:00Z"
	var buf bytes.Buffer
	renderRunsTable(&buf, []api.RunSummary{
		{
			ID:             "run-1",
			Status:         api.RunStatus{Result: "succeeded", Execution: "finished"},
			RepositoryName: "rwx-cloud/cloud",
			Branch:         "main",
			CommitSha:      "abcdef1234567890",
			DefinitionPath: ".rwx/ci.yml",
			Title:          "CI",
			CreatedAt:      &created,
		},
	})

	out := buf.String()
	require.Contains(t, out, "ID")
	require.Contains(t, out, "STATUS")
	require.Contains(t, out, "CREATED")
	require.Contains(t, out, "run-1")
	require.Contains(t, out, "succeeded")
	require.Contains(t, out, "abcdef1") // commit truncated to 7 chars
	require.NotContains(t, out, "abcdef1234567890")
	require.Contains(t, out, "2026-06-23T12:00:00Z")
}

func TestRenderRunsTableTruncatesTitleAndBranch(t *testing.T) {
	longTitle := "This is an extremely long run title that should be truncated in the table"
	longBranch := "feature/some-really-long-branch-name-that-overflows"
	var buf bytes.Buffer
	renderRunsTable(&buf, []api.RunSummary{
		{ID: "run-1", Title: longTitle, Branch: longBranch},
	})

	out := buf.String()
	require.NotContains(t, out, longTitle)
	require.NotContains(t, out, longBranch)
	require.Contains(t, out, "…")
	// The truncated cells keep their leading content.
	require.Contains(t, out, "This is an extremely long run title")
	require.Contains(t, out, "feature/some-really")
}

func TestRunStatusLabel(t *testing.T) {
	// While executing, the execution phase is the meaningful state.
	require.Equal(t, "in_progress", runStatusLabel(api.RunStatus{Result: "no_result", Execution: "in_progress"}))
	// Once finished, the result is.
	require.Equal(t, "succeeded", runStatusLabel(api.RunStatus{Result: "succeeded", Execution: "finished"}))
}

func TestRunsJSONUsesPascalCaseAndIncludesNextCursor(t *testing.T) {
	next := "next-cursor"
	encoded, err := runsJSON(&api.ListRunsResult{
		Runs: []api.RunSummary{
			{
				ID:             "run-1",
				Status:         api.RunStatus{Result: "succeeded", Execution: "finished"},
				RepositoryName: "rwx-cloud/cloud",
				RunURL:         "https://cloud.rwx.com/runs/run-1",
			},
		},
		Pagination: api.ListRunsPagination{NextCursor: &next, Limit: 50},
	})
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(encoded, &decoded))

	runs, ok := decoded["Runs"].([]any)
	require.True(t, ok, "expected PascalCase Runs key")
	run := runs[0].(map[string]any)
	require.Equal(t, "run-1", run["ID"])
	require.Equal(t, "rwx-cloud/cloud", run["RepositoryName"])
	// pascalCaseKeys only treats "id" as an initialism, so run_url -> RunUrl. This
	// matches the casing 'rwx results --output json' emits via the same helper;
	// pin it so the shared convention can't drift unnoticed.
	require.Equal(t, "https://cloud.rwx.com/runs/run-1", run["RunUrl"])
	status := run["Status"].(map[string]any)
	require.Equal(t, "succeeded", status["Result"])
	require.Equal(t, "finished", status["Execution"])

	pagination := decoded["Pagination"].(map[string]any)
	require.Equal(t, "next-cursor", pagination["NextCursor"])
}

func TestPrintRunFilterSuggestions(t *testing.T) {
	var buf bytes.Buffer
	printRunFilterSuggestions(&buf, map[string][]api.RunFilterSuggestion{
		"branch_names": {{Value: "develp", Suggestions: []string{"develop"}}},
	})

	out := buf.String()
	require.Contains(t, out, "develp")
	require.Contains(t, out, "develop")
}
