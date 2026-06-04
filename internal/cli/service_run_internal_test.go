package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunPatchPathspec(t *testing.T) {
	t.Run("excludes rwx runtime directories", func(t *testing.T) {
		wd := t.TempDir()
		t.Chdir(wd)
		require.NoError(t, os.Mkdir(filepath.Join(wd, ".rwx"), 0o755))

		pathspec := runPatchPathspec(".rwx/sandbox.yml", filepath.Join(wd, ".rwx"))

		require.Equal(t, []string{
			".",
			":!.rwx/sandbox.yml",
			":!.rwx/sandboxes",
			":!.rwx/downloads",
			":!.rwx/test-suites",
		}, pathspec)
	})

	t.Run("uses configured rwx directory", func(t *testing.T) {
		wd := t.TempDir()
		t.Chdir(wd)
		require.NoError(t, os.Mkdir(filepath.Join(wd, "ci"), 0o755))

		pathspec := runPatchPathspec("workflow.yml", filepath.Join(wd, "ci"))

		require.Equal(t, []string{
			".",
			":!workflow.yml",
			":!ci/sandboxes",
			":!ci/downloads",
			":!ci/test-suites",
		}, pathspec)
	})

	t.Run("skips runtime directory exclusions outside the working directory", func(t *testing.T) {
		wd := t.TempDir()
		t.Chdir(wd)

		pathspec := runPatchPathspec("workflow.yml", filepath.Join(t.TempDir(), ".rwx"))

		require.Equal(t, []string{".", ":!workflow.yml"}, pathspec)
	})
}
