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

// isolate keeps git and jj from reading the developer's real configuration. The
// temp dir is resolved because macOS puts it behind a /var -> /private/var
// symlink, which git reports in its resolved form.
func isolate(t *testing.T) string {
	t.Helper()

	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	jjConfig := filepath.Join(dir, "jj-config.toml")
	require.NoError(t, os.WriteFile(jjConfig, nil, 0o644))

	t.Setenv("JJ_USER", "Test User")
	t.Setenv("JJ_EMAIL", "test@example.com")
	t.Setenv("JJ_CONFIG", jjConfig)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)

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

func TestNewSelectsJJForColocatedRepositories(t *testing.T) {
	root := jjRepo(t, true)
	require.DirExists(t, filepath.Join(root, ".git"), "colocated repos keep a top-level .git")
	require.IsType(t, &jj.Client{}, vcs.New(root))
}

func TestNewSelectsJJForNonColocatedRepositories(t *testing.T) {
	root := jjRepo(t, false)
	require.NoDirExists(t, filepath.Join(root, ".git"), "non-colocated repos have no top-level .git")
	require.IsType(t, &jj.Client{}, vcs.New(root))
}

// git rev-parse walks upward, so an enclosing checkout would answer for a
// nested jj repo that has no .git of its own.
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

func TestNewPrefersAGitRepositoryNestedInAJJWorkspace(t *testing.T) {
	outer := jjRepo(t, false)

	nested := filepath.Join(outer, "nested")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	mustRun(t, nested, "git", "init", "-q")

	require.IsType(t, &git.Client{}, vcs.New(nested))
	require.IsType(t, &jj.Client{}, vcs.New(outer))
}

func TestNewSelectsGitForGitRepositories(t *testing.T) {
	root := isolate(t)
	mustRun(t, root, "git", "init", "-q")

	require.IsType(t, &git.Client{}, vcs.New(root))
}

func TestNewResolvesTheClientFromASubdirectory(t *testing.T) {
	root := isolate(t)
	mustRun(t, root, "git", "init", "-q")

	nested := filepath.Join(root, "nested")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	client := vcs.New(nested)
	require.IsType(t, &git.Client{}, client)
	require.Equal(t, root, client.GetTopLevel())
}

func TestNewSelectsGitOutsideAnyRepository(t *testing.T) {
	require.IsType(t, &git.Client{}, vcs.New(isolate(t)))
}
