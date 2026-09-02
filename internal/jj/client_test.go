package jj_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/jj"
	"github.com/stretchr/testify/require"
)

func requireJJ(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
}

type fixture struct {
	tempDir string
	root    string
	origin  string
	baseSHA string
	client  *jj.Client
}

// isolateVCSEnv keeps jj and git from reading the developer's real
// configuration. jj gets an empty config file rather than /dev/null, whose
// handling as a character device varies.
func isolateVCSEnv(t *testing.T, dir string) {
	t.Helper()

	jjConfig := filepath.Join(dir, "jj-config.toml")
	require.NoError(t, os.WriteFile(jjConfig, nil, 0o644))

	t.Setenv("JJ_USER", "Test User")
	t.Setenv("JJ_EMAIL", "test@example.com")
	t.Setenv("JJ_CONFIG", jjConfig)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_AUTHOR_NAME", "Test User")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test User")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	t.Setenv("RWX_GIT_REMOTE", "")
	t.Setenv("RWX_VCS", "")
}

func newClient(dir string) *jj.Client {
	return &jj.Client{Binary: "jj", GitBinary: "git", Dir: dir}
}

// repoFixture runs the named testdata script in a scratch directory and returns
// the repository it built. Reading the script also registers it with the go test
// cache, so editing a fixture invalidates any cached result.
func repoFixture(t *testing.T, name string, colocated bool) fixture {
	t.Helper()
	requireJJ(t)

	tempDir := t.TempDir()
	isolateVCSEnv(t, tempDir)

	colocate := "--no-colocate"
	if colocated {
		colocate = "--colocate"
	}
	t.Setenv("COLOCATE", colocate)

	script, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, name), script, 0o755))

	var stdout, stderr bytes.Buffer
	cmd := exec.Command("bash", name)
	cmd.Dir = tempDir
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("fixture %s failed: %v\nstdout: %s\nstderr: %s", name, err, &stdout, &stderr)
	}

	root := filepath.Join(tempDir, "repo")
	return fixture{
		tempDir: tempDir,
		root:    root,
		origin:  filepath.Join(tempDir, "origin"),
		baseSHA: lastWord(stdout.String()),
		client:  newClient(root),
	}
}

// lastWord reads the base commit a fixture printed, or "" when it printed none.
func lastWord(out string) string {
	words := strings.Fields(out)
	if len(words) == 0 {
		return ""
	}
	return words[len(words)-1]
}

// eachLayout runs fn against both jj layouts: a workspace with its own git store
// and a colocated one sharing a .git with the working copy.
func eachLayout(t *testing.T, fixturePath string, fn func(t *testing.T, f fixture)) {
	t.Helper()

	for _, layout := range []struct {
		name      string
		colocated bool
	}{
		{"non-colocated", false},
		{"colocated", true},
	} {
		t.Run(layout.name, func(t *testing.T) {
			fn(t, repoFixture(t, fixturePath, layout.colocated))
		})
	}
}

func requireSamePath(t *testing.T, want, got string) {
	t.Helper()

	resolvedWant, err := filepath.EvalSymlinks(want)
	require.NoError(t, err)
	resolvedGot, err := filepath.EvalSymlinks(got)
	require.NoError(t, err)
	require.Equal(t, resolvedWant, resolvedGot)
}

func TestGetTopLevel(t *testing.T) {
	t.Run("finds the workspace root", func(t *testing.T) {
		eachLayout(t, "clone", func(t *testing.T, f fixture) {
			requireSamePath(t, f.root, f.client.GetTopLevel())
			require.True(t, f.client.IsInsideWorkTree())
			require.Empty(t, f.client.MissingDependency())
		})
	})

	t.Run("finds it from a subdirectory", func(t *testing.T) {
		eachLayout(t, "clone", func(t *testing.T, f fixture) {
			requireSamePath(t, f.root, newClient(filepath.Join(f.root, "nested")).GetTopLevel())
		})
	})

	t.Run("is empty outside a repository", func(t *testing.T) {
		requireJJ(t)

		client := newClient(t.TempDir())
		require.False(t, client.IsInsideWorkTree())
		require.Empty(t, client.GetTopLevel())
	})
}

func TestMissingDependency(t *testing.T) {
	requireJJ(t)

	require.Equal(t, "jj", (&jj.Client{Binary: "jj-not-installed", GitBinary: "git"}).MissingDependency())
	require.Equal(t, "git", (&jj.Client{Binary: "jj", GitBinary: "git-not-installed"}).MissingDependency())
}

func TestGetHead(t *testing.T) {
	eachLayout(t, "clone", func(t *testing.T, f fixture) {
		head, err := f.client.GetHeadCommit()
		require.NoError(t, err)
		require.Len(t, head, 40)

		short := f.client.GetShortHead()
		require.Len(t, short, 7)
		require.NotEqual(t, head[:7], short, "GetShortHead reports a change id, not a commit id")
	})
}

func TestGetHeadCommitTracksWorkingCopy(t *testing.T) {
	eachLayout(t, "clone", func(t *testing.T, f fixture) {
		before, err := f.client.GetHeadCommit()
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(f.root, "base.txt"), []byte("hello\nchanged\n"), 0o644))

		after, err := f.client.GetHeadCommit()
		require.NoError(t, err)
		require.NotEqual(t, before, after, "jj snapshots the working copy into @")
	})
}

// GetShortHead keys sandbox sessions, so it has to survive @ being rewritten.
func TestGetShortHeadSurvivesWorkingCopyEdits(t *testing.T) {
	eachLayout(t, "clone", func(t *testing.T, f fixture) {
		beforeCommit, err := f.client.GetHeadCommit()
		require.NoError(t, err)
		beforeShort := f.client.GetShortHead()
		require.NotEmpty(t, beforeShort)

		require.NoError(t, os.WriteFile(filepath.Join(f.root, "base.txt"), []byte("hello\nedited\n"), 0o644))

		afterCommit, err := f.client.GetHeadCommit()
		require.NoError(t, err)
		require.NotEqual(t, beforeCommit, afterCommit, "jj rewrites @ on every snapshot")
		require.Equal(t, beforeShort, f.client.GetShortHead(), "the session key must survive the rewrite")

		// The rewritten commit is not a descendant of the one it replaced, which
		// is exactly why the commit id cannot key the session...
		require.False(t, f.client.IsAncestor(beforeCommit, afterCommit))
		// ...while the change id still resolves to the current working copy, so
		// the sandbox ancestry fallback keeps matching the existing session.
		require.True(t, f.client.IsAncestor(beforeShort, "HEAD"))
	})
}

func TestIsAncestor(t *testing.T) {
	eachLayout(t, "clone-local-work", func(t *testing.T, f fixture) {
		head, err := f.client.GetHeadCommit()
		require.NoError(t, err)

		require.True(t, f.client.IsAncestor(f.baseSHA, head))
		require.True(t, f.client.IsAncestor(f.baseSHA, "HEAD"))
		require.False(t, f.client.IsAncestor(head, f.baseSHA))
		require.False(t, f.client.IsAncestor("deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", head))
		require.False(t, f.client.IsAncestor("", head))
		require.False(t, f.client.IsAncestor(f.baseSHA, ""))
	})
}
