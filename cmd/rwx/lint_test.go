package main_test

import (
	"testing"

	rwx "github.com/rwx-cloud/rwx/cmd/rwx"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestLintShouldFail(t *testing.T) {
	t.Run("clean result without warnings-as-errors", func(t *testing.T) {
		result := &cli.LintResult{HasError: false, WarningCount: 0}
		require.False(t, rwx.LintShouldFail(result, false))
	})

	t.Run("clean result with warnings-as-errors", func(t *testing.T) {
		result := &cli.LintResult{HasError: false, WarningCount: 0}
		require.False(t, rwx.LintShouldFail(result, true))
	})

	t.Run("warnings without warnings-as-errors", func(t *testing.T) {
		result := &cli.LintResult{HasError: false, WarningCount: 2}
		require.False(t, rwx.LintShouldFail(result, false))
	})

	t.Run("warnings with warnings-as-errors", func(t *testing.T) {
		result := &cli.LintResult{HasError: false, WarningCount: 2}
		require.True(t, rwx.LintShouldFail(result, true))
	})

	t.Run("errors without warnings-as-errors", func(t *testing.T) {
		result := &cli.LintResult{HasError: true, ErrorCount: 1}
		require.True(t, rwx.LintShouldFail(result, false))
	})

	t.Run("errors with warnings-as-errors", func(t *testing.T) {
		result := &cli.LintResult{HasError: true, ErrorCount: 1, WarningCount: 1}
		require.True(t, rwx.LintShouldFail(result, true))
	})
}
