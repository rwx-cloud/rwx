package main_test

import (
	"testing"

	rwx "github.com/rwx-cloud/rwx/cmd/rwx"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/stretchr/testify/require"
)

func TestHandleAmbiguousDefinitionPathError(t *testing.T) {
	t.Run("formats suggested commands with explicit branch and repo", func(t *testing.T) {
		err := &api.AmbiguousDefinitionPathError{
			Message:                 "Multiple definitions found",
			MatchingDefinitionPaths: []string{".rwx/ci.yml", ".rwx/deploy.yml"},
		}

		result := rwx.HandleAmbiguousDefinitionPathError(err, "main", "cloud")
		require.Error(t, result)
		require.True(t, errors.Is(result, errors.ErrAmbiguousDefinitionPath))
		require.Contains(t, result.Error(), "rwx results --branch main --repo cloud --definition .rwx/ci.yml")
		require.Contains(t, result.Error(), "rwx results --branch main --repo cloud --definition .rwx/deploy.yml")
	})

	t.Run("omits branch and repo when not explicitly provided", func(t *testing.T) {
		err := &api.AmbiguousDefinitionPathError{
			Message:                 "Multiple definitions found",
			MatchingDefinitionPaths: []string{".rwx/ci.yml", ".rwx/deploy.yml"},
		}

		result := rwx.HandleAmbiguousDefinitionPathError(err, "", "")
		require.Error(t, result)
		require.Contains(t, result.Error(), "rwx results --definition .rwx/ci.yml")
		require.Contains(t, result.Error(), "rwx results --definition .rwx/deploy.yml")
		require.NotContains(t, result.Error(), "--branch")
		require.NotContains(t, result.Error(), "--repo")
	})

	t.Run("includes only branch when repo is inferred", func(t *testing.T) {
		err := &api.AmbiguousDefinitionPathError{
			Message:                 "Multiple definitions found",
			MatchingDefinitionPaths: []string{".rwx/ci.yml"},
		}

		result := rwx.HandleAmbiguousDefinitionPathError(err, "main", "")
		require.Error(t, result)
		require.Contains(t, result.Error(), "rwx results --branch main --definition .rwx/ci.yml")
		require.NotContains(t, result.Error(), "--repo")
	})

	t.Run("includes only repo when branch is inferred", func(t *testing.T) {
		err := &api.AmbiguousDefinitionPathError{
			Message:                 "Multiple definitions found",
			MatchingDefinitionPaths: []string{".rwx/ci.yml"},
		}

		result := rwx.HandleAmbiguousDefinitionPathError(err, "", "cloud")
		require.Error(t, result)
		require.Contains(t, result.Error(), "rwx results --repo cloud --definition .rwx/ci.yml")
		require.NotContains(t, result.Error(), "--branch")
	})

	t.Run("preserves server error message", func(t *testing.T) {
		err := &api.AmbiguousDefinitionPathError{
			Message:                 "Multiple definitions found for cloud on branch main. Specify a definition using --definition, e.g.:",
			MatchingDefinitionPaths: []string{".rwx/ci.yml"},
		}

		result := rwx.HandleAmbiguousDefinitionPathError(err, "", "")
		require.Contains(t, result.Error(), "Multiple definitions found for cloud on branch main")
	})

	t.Run("passes through non-ambiguous errors unchanged", func(t *testing.T) {
		err := errors.New("some other error")

		result := rwx.HandleAmbiguousDefinitionPathError(err, "main", "cloud")
		require.Equal(t, err, result)
	})
}

func TestMergeEnrichedResults(t *testing.T) {
	t.Run("PascalCases enriched keys, including nested maps and slices", func(t *testing.T) {
		base := map[string]any{"RunID": "abc123"}
		details := map[string]any{
			"started_at": "2026-06-04T17:02:11Z",
			"status":     map[string]any{"finished_status": "failed"},
			"tasks": []any{
				map[string]any{"id": "task_def456", "completed_runtime_seconds": float64(250)},
			},
		}

		merged := rwx.MergeEnrichedResults(base, details)

		require.Equal(t, "abc123", merged["RunID"])
		require.Equal(t, "2026-06-04T17:02:11Z", merged["StartedAt"])
		require.Equal(t, map[string]any{"FinishedStatus": "failed"}, merged["Status"])
		require.Equal(t, []any{map[string]any{"ID": "task_def456", "CompletedRuntimeSeconds": float64(250)}}, merged["Tasks"])
	})

	t.Run("keeps the enriched id as ID alongside the base id keys", func(t *testing.T) {
		base := map[string]any{"RunID": "abc123"}
		details := map[string]any{"id": "abc123", "branch": "main"}

		merged := rwx.MergeEnrichedResults(base, details)

		require.Equal(t, "abc123", merged["ID"])
		require.Equal(t, "abc123", merged["RunID"])
		require.Equal(t, "main", merged["Branch"])
	})

	t.Run("preserves base keys on collision", func(t *testing.T) {
		base := map[string]any{"Branch": "base-value"}
		details := map[string]any{"branch": "enriched-value"}

		merged := rwx.MergeEnrichedResults(base, details)

		require.Equal(t, "base-value", merged["Branch"])
	})

	t.Run("returns base unchanged for empty details", func(t *testing.T) {
		base := map[string]any{"RunID": "abc123", "ResultStatus": "failed"}

		merged := rwx.MergeEnrichedResults(base, map[string]any{})

		require.Equal(t, map[string]any{"RunID": "abc123", "ResultStatus": "failed"}, merged)
	})
}
