package vcs_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rwx-cloud/rwx/internal/git"
	"github.com/rwx-cloud/rwx/internal/jj"
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

func isolate(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	jjConfig := filepath.Join(dir, "jj-config.toml")
	require.NoError(t, os.WriteFile(jjConfig, nil, 0o644))

	t.Setenv("JJ_USER", "Test User")
	t.Setenv("JJ_EMAIL", "test@example.com")
	t.Setenv("JJ_CONFIG", jjConfig)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("RWX_VCS", "")

	return dir
}

func jjRepo(t *testing.T, colocated bool) string {
	t.Helper()

	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed")
	}

	root := filepath.Join(isolate(t), "repo")
	require.NoError(t, os.MkdirAll(root, 0o755))

	args := []string{"--no-pager", "--quiet", "git", "init"}
	if colocated {
		args = append(args, "--colocate")
	} else {
		args = append(args, "--no-colocate")
	}
	mustRun(t, root, "jj", args...)

	return root
}

func TestNewPrefersGitForColocatedRepositories(t *testing.T) {
	root := jjRepo(t, true)
	require.DirExists(t, filepath.Join(root, ".git"), "colocated repos keep a top-level .git")
	require.IsType(t, &git.Client{}, vcs.New(root))
}

func TestNewSelectsJJForNonColocatedRepositories(t *testing.T) {
	root := jjRepo(t, false)
	require.NoDirExists(t, filepath.Join(root, ".git"), "non-colocated repos have no top-level .git")
	require.IsType(t, &jj.Client{}, vcs.New(root))
}

// git rev-parse walks upward, so an enclosing checkout answers for a nested jj
// repo that has no .git of its own. The innermost root has to win, or the
// backend would report the wrong repository entirely.
func TestNewPrefersTheInnermostRepository(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed")
	}

	outer := isolate(t)
	mustRun(t, outer, "git", "init", "-q")

	nested := filepath.Join(outer, "nested")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	mustRun(t, nested, "jj", "--no-pager", "--quiet", "git", "init", "--no-colocate")

	require.NoDirExists(t, filepath.Join(nested, ".git"))
	require.IsType(t, &jj.Client{}, vcs.New(nested))

	// The enclosing checkout still selects git for its own directories.
	require.IsType(t, &git.Client{}, vcs.New(outer))
}

func TestNewSelectsGitForGitRepositories(t *testing.T) {
	root := isolate(t)
	mustRun(t, root, "git", "init", "-q")

	require.IsType(t, &git.Client{}, vcs.New(root))
}

func TestNewSelectsGitOutsideAnyRepository(t *testing.T) {
	require.IsType(t, &git.Client{}, vcs.New(isolate(t)))
}

func TestNewHonorsRWXVCS(t *testing.T) {
	t.Run("jj opts in for a colocated repository", func(t *testing.T) {
		root := jjRepo(t, true)

		t.Setenv("RWX_VCS", "jj")
		require.IsType(t, &jj.Client{}, vcs.New(root))
	})

	t.Run("git forces git for a non-colocated repository", func(t *testing.T) {
		root := jjRepo(t, false)

		t.Setenv("RWX_VCS", "git")
		require.IsType(t, &git.Client{}, vcs.New(root))
	})

	t.Run("is case and whitespace insensitive", func(t *testing.T) {
		root := jjRepo(t, true)

		t.Setenv("RWX_VCS", "  JJ ")
		require.IsType(t, &jj.Client{}, vcs.New(root))
	})

	t.Run("an unknown value falls back to detection", func(t *testing.T) {
		root := jjRepo(t, false)

		t.Setenv("RWX_VCS", "hg")
		require.IsType(t, &jj.Client{}, vcs.New(root))
	})
}
