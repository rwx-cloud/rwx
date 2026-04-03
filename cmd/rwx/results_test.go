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
