package vcs_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rwx-cloud/rwx/internal/git"
	"github.com/rwx-cloud/rwx/internal/vcs"
	"github.com/stretchr/testify/require"
)

func mustRun(t *testing.T, dir, bin string, args ...string) {
	t.Helper()

	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s failed: %s", bin, out)
}

// isolate keeps git from reading the developer's real configuration. The temp
// dir is resolved because macOS puts it behind a /var -> /private/var symlink,
// which git reports in its resolved form.
func isolate(t *testing.T) string {
	t.Helper()

	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)

	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	return dir
}

func TestNewSelectsGitForGitRepositories(t *testing.T) {
	root := isolate(t)
	mustRun(t, root, "git", "init", "-q")

	client := vcs.New(root)
	require.IsType(t, &git.Client{}, client)
	require.Equal(t, root, client.GetTopLevel())
}

func TestNewSelectsGitOutsideAnyRepository(t *testing.T) {
	root := isolate(t)

	client := vcs.New(root)
	require.IsType(t, &git.Client{}, client)
	require.Empty(t, client.GetTopLevel())
	require.False(t, client.IsInsideWorkTree())
}

func TestNewResolvesTheClientFromASubdirectory(t *testing.T) {
	root := isolate(t)
	mustRun(t, root, "git", "init", "-q")

	nested := filepath.Join(root, "nested")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	require.Equal(t, root, vcs.New(nested).GetTopLevel())
}
