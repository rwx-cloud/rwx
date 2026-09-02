package jj_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/jj"
	"github.com/rwx-cloud/rwx/internal/vcs/vcstypes"
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
		require.Empty(t, client.GetBranch())
	})
}

func TestMissingDependency(t *testing.T) {
	requireJJ(t)

	require.Equal(t, "jj", (&jj.Client{Binary: "jj-not-installed", GitBinary: "git"}).MissingDependency())
	require.Equal(t, "git", (&jj.Client{Binary: "jj", GitBinary: "git-not-installed"}).MissingDependency())
}

func TestGetBranch(t *testing.T) {
	t.Run("returns the bookmark on the working copy", func(t *testing.T) {
		eachLayout(t, "clone-bookmark-on-working-copy", func(t *testing.T, f fixture) {
			require.Equal(t, "feature", f.client.GetBranch())
		})
	})

	// `main` stays on the base commit as @ moves ahead of it.
	t.Run("returns the nearest bookmark below the working copy", func(t *testing.T) {
		eachLayout(t, "clone-local-work", func(t *testing.T, f fixture) {
			require.Equal(t, "main", f.client.GetBranch())
		})
	})

	// `feature` sits on the described change, closer to @ than `main`.
	t.Run("prefers the closest bookmark", func(t *testing.T) {
		eachLayout(t, "clone-bookmark-on-ancestor", func(t *testing.T, f fixture) {
			require.Equal(t, "feature", f.client.GetBranch())
		})
	})

	// @ merges two bookmarked lines, neither closer than the other. `aaa-local`
	// sorts first but only `topic` exists on the remote.
	t.Run("prefers a bookmark that exists on the remote when the nearest are tied", func(t *testing.T) {
		eachLayout(t, "clone-merged-bookmarks", func(t *testing.T, f fixture) {
			require.Equal(t, "topic", f.client.GetBranch())

			t.Setenv("RWX_GIT_REMOTE", "nonexistent")
			require.Equal(t, "aaa-local", f.client.GetBranch(), "falls back to name order without a remote match")
		})
	})

	t.Run("is empty when the repository has no bookmarks", func(t *testing.T) {
		eachLayout(t, "init-local-commit", func(t *testing.T, f fixture) {
			require.Empty(t, f.client.GetBranch())
		})
	})
}

func TestGetRemoteUrl(t *testing.T) {
	t.Run("resolves origin", func(t *testing.T) {
		eachLayout(t, "clone", func(t *testing.T, f fixture) {
			requireSamePath(t, f.origin, f.client.GetOriginUrl())
			requireSamePath(t, f.origin, f.client.GetRemoteUrl("origin"))
			require.Empty(t, f.client.GetRemoteUrl("upstream"))
		})
	})

	t.Run("picks origin out of several remotes", func(t *testing.T) {
		eachLayout(t, "clone-many-remotes", func(t *testing.T, f fixture) {
			requireSamePath(t, f.origin, f.client.GetOriginUrl())
			require.NotEmpty(t, f.client.GetRemoteUrl("upstream"))
		})
	})

	t.Run("is empty without any remote", func(t *testing.T) {
		eachLayout(t, "init-uncommitted", func(t *testing.T, f fixture) {
			require.Empty(t, f.client.GetOriginUrl())
		})
	})

	t.Run("is empty when no remote is named origin", func(t *testing.T) {
		eachLayout(t, "clone-remote-named-upstream", func(t *testing.T, f fixture) {
			require.Empty(t, f.client.GetOriginUrl())
			require.NotEmpty(t, f.client.GetRemoteUrl("upstream"))
		})
	})

	t.Run("honors RWX_GIT_REMOTE", func(t *testing.T) {
		eachLayout(t, "clone-remote-named-upstream", func(t *testing.T, f fixture) {
			t.Setenv("RWX_GIT_REMOTE", "upstream")

			requireSamePath(t, f.origin, f.client.GetOriginUrl())

			sha, err := f.client.GetCommit()
			require.NoError(t, err)
			require.Equal(t, f.baseSHA, sha)
		})
	})
}

func headCommit(t *testing.T, client *jj.Client) string {
	t.Helper()

	head, err := client.GetHeadCommit()
	require.NoError(t, err)
	return head
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

func TestGetCommit(t *testing.T) {
	t.Run("returns the remote commit when the working copy is clean", func(t *testing.T) {
		eachLayout(t, "clone", func(t *testing.T, f fixture) {
			sha, err := f.client.GetCommit()
			require.NoError(t, err)
			require.Equal(t, f.baseSHA, sha)
		})
	})

	t.Run("returns the remote commit when ahead of the remote", func(t *testing.T) {
		eachLayout(t, "clone-local-work", func(t *testing.T, f fixture) {
			sha, err := f.client.GetCommit()
			require.NoError(t, err)
			require.Equal(t, f.baseSHA, sha)
		})
	})

	t.Run("returns the shared commit when behind the remote", func(t *testing.T) {
		eachLayout(t, "clone-behind-origin", func(t *testing.T, f fixture) {
			sha, err := f.client.GetCommit()
			require.NoError(t, err)
			require.Equal(t, f.baseSHA, sha)
		})
	})

	t.Run("returns the fork point when both sides have moved on", func(t *testing.T) {
		eachLayout(t, "clone-diverged-from-origin", func(t *testing.T, f fixture) {
			sha, err := f.client.GetCommit()
			require.NoError(t, err)
			require.Equal(t, f.baseSHA, sha)
		})
	})

	t.Run("returns the fork point for work left behind a bookmark", func(t *testing.T) {
		eachLayout(t, "clone-bookmark-on-ancestor", func(t *testing.T, f fixture) {
			sha, err := f.client.GetCommit()
			require.NoError(t, err)
			require.Equal(t, f.baseSHA, sha)
		})
	})

	t.Run("falls back to the working copy in a repository with no remotes", func(t *testing.T) {
		eachLayout(t, "init-uncommitted", func(t *testing.T, f fixture) {
			sha, err := f.client.GetCommit()
			require.NoError(t, err)
			require.Equal(t, headCommit(t, f.client), sha)
		})
	})

	// No bookmark below @ is jj's detached HEAD, where the git backend answers
	// with HEAD rather than erroring. Never the virtual root.
	t.Run("falls back to the working copy without a common ancestor", func(t *testing.T) {
		eachLayout(t, "init-unrelated-origin", func(t *testing.T, f fixture) {
			sha, err := f.client.GetCommit()
			require.NoError(t, err)
			require.Equal(t, headCommit(t, f.client), sha)
			require.NotEqual(t, strings.Repeat("0", 40), sha)
		})
	})

	// A bookmark below @ is a named position, so the misconfiguration is
	// reported, as git does on a branch.
	t.Run("errors without a common ancestor when a bookmark names the change", func(t *testing.T) {
		eachLayout(t, "init-unrelated-origin-bookmark", func(t *testing.T, f fixture) {
			sha, err := f.client.GetCommit()
			require.ErrorContains(t, err, "no commits in common")
			require.ErrorContains(t, err, "feature")
			require.Empty(t, sha)
		})
	})

	t.Run("errors on a bookmarked line when no remote is named origin", func(t *testing.T) {
		eachLayout(t, "clone-remote-named-upstream", func(t *testing.T, f fixture) {
			_, err := f.client.GetCommit()
			require.ErrorContains(t, err, "no git remote named 'origin' is configured")
		})
	})
}

// staleHead reads @ without letting jj snapshot.
func staleHead(t *testing.T, root string) string {
	t.Helper()

	cmd := exec.Command("jj", "--no-pager", "--color=never", "--quiet", "--ignore-working-copy",
		"log", "-r", "@", "--no-graph", "-T", "commit_id")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "jj log failed: %s", out)
	return strings.TrimSpace(string(out))
}

// Reporting commands such as `rwx results` call GetCommit and nothing else, so
// it must not rewrite @.
func TestGetCommitDoesNotSnapshotTheWorkingCopy(t *testing.T) {
	eachLayout(t, "clone-local-work", func(t *testing.T, f fixture) {
		_, err := f.client.GetHeadCommit()
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(f.root, "base.txt"), []byte("uncommitted\n"), 0o644))
		before := staleHead(t, f.root)

		base, err := f.client.GetCommit()
		require.NoError(t, err)
		require.Equal(t, f.baseSHA, base)
		require.Equal(t, before, staleHead(t, f.root), "GetCommit must leave @ alone")

		// The base is unchanged by the snapshot, which is what makes the stale
		// read safe.
		head, err := f.client.GetHeadCommit()
		require.NoError(t, err)
		require.NotEqual(t, before, head, "GetHeadCommit does snapshot")

		after, err := f.client.GetCommit()
		require.NoError(t, err)
		require.Equal(t, base, after)
	})
}

func TestGeneratePatch(t *testing.T) {
	t.Run("covers local commits and working copy changes", func(t *testing.T) {
		eachLayout(t, "clone-local-work", func(t *testing.T, f fixture) {
			patch, lfs, err := f.client.GeneratePatch(nil)
			require.NoError(t, err)
			require.Nil(t, lfs)

			text := string(patch)
			require.Contains(t, text, "+committed")
			require.Contains(t, text, "+working copy")
			require.Contains(t, text, "b/added.txt")
			require.Contains(t, text, "b/nested/more/x.txt")
			require.Contains(t, text, "a/sub.txt")
			require.Contains(t, text, "deleted file mode")
		})
	})

	t.Run("includes local commits with a clean working copy", func(t *testing.T) {
		eachLayout(t, "clone-local-commit", func(t *testing.T, f fixture) {
			patch, _, err := f.client.GeneratePatch(nil)
			require.NoError(t, err)
			require.Contains(t, string(patch), "+committed")
		})
	})

	t.Run("includes files that only exist in the working copy", func(t *testing.T) {
		eachLayout(t, "clone-new-file", func(t *testing.T, f fixture) {
			patch, _, err := f.client.GeneratePatch(nil)
			require.NoError(t, err)
			require.Contains(t, string(patch), "b/added.txt")
			require.Contains(t, string(patch), "+brand new")
		})
	})

	t.Run("is empty when the working copy matches the remote", func(t *testing.T) {
		eachLayout(t, "clone", func(t *testing.T, f fixture) {
			patch, lfs, err := f.client.GeneratePatch(nil)
			require.NoError(t, err)
			require.Nil(t, lfs)
			require.Empty(t, patch)
		})
	})

	t.Run("omits ignored files", func(t *testing.T) {
		eachLayout(t, "clone-ignored-file", func(t *testing.T, f fixture) {
			patch, _, err := f.client.GeneratePatch(nil)
			require.NoError(t, err)
			require.Contains(t, string(patch), "+tracked change")
			require.NotContains(t, string(patch), "b/ignored.txt")
			require.NotContains(t, string(patch), "do not send")
		})
	})

	t.Run("honors pathspec exclusions", func(t *testing.T) {
		eachLayout(t, "clone-local-work", func(t *testing.T, f fixture) {
			patch, _, err := f.client.GeneratePatch([]string{":/", ":(top,exclude)added.txt"})
			require.NoError(t, err)
			require.NotContains(t, string(patch), "b/added.txt")
			require.Contains(t, string(patch), "b/base.txt")
		})
	})

	t.Run("keeps paths relative to the repository root", func(t *testing.T) {
		eachLayout(t, "clone-local-work", func(t *testing.T, f fixture) {
			patch, _, err := newClient(filepath.Join(f.root, "nested")).GeneratePatch(nil)
			require.NoError(t, err)
			require.Contains(t, string(patch), "b/base.txt")
			require.Contains(t, string(patch), "b/nested/more/x.txt")
		})
	})

	t.Run("reports LFS-tracked changes instead of a patch", func(t *testing.T) {
		eachLayout(t, "clone-lfs-file", func(t *testing.T, f fixture) {
			patch, lfs, err := f.client.GeneratePatch(nil)
			require.NoError(t, err)
			require.Nil(t, patch)
			require.NotNil(t, lfs)
			require.Equal(t, 1, lfs.Count)
			require.Equal(t, []string{"seed.bin"}, lfs.Files)
		})
	})

	t.Run("refuses a conflicted working copy", func(t *testing.T) {
		eachLayout(t, "clone-conflicted-merge", func(t *testing.T, f fixture) {
			patch, lfs, err := f.client.GeneratePatch(nil)
			require.NoError(t, err)
			require.Nil(t, lfs)
			require.Empty(t, patch, "nothing from the conflicted store may leak into a patch")

			_, err = f.client.GeneratePatchFile(filepath.Join(f.tempDir, "patches"), nil)
			require.ErrorContains(t, err, "unresolved conflicts")
			require.ErrorContains(t, err, "jj resolve")
		})
	})
}

func TestGeneratePatchRoundTripsThroughGitApply(t *testing.T) {
	eachLayout(t, "clone-local-work-binary", func(t *testing.T, f fixture) {
		readWorkingCopy := func(rel string) []byte {
			contents, err := os.ReadFile(filepath.Join(f.root, rel))
			require.NoError(t, err)
			return contents
		}
		wantModifiedBinary := readWorkingCopy("seed.bin")
		wantNewBinary := readWorkingCopy("fresh.bin")

		patch, lfs, err := f.client.GeneratePatch(nil)
		require.NoError(t, err)
		require.Nil(t, lfs)
		require.Contains(t, string(patch), "GIT binary patch", "binary files need literal patch data to survive")

		verify := filepath.Join(f.tempDir, "verify")
		out, err := exec.Command("git", "clone", "-q", f.origin, verify).CombinedOutput()
		require.NoError(t, err, "git clone failed: %s", out)

		checkout := exec.Command("git", "checkout", "-q", f.baseSHA)
		checkout.Dir = verify
		out, err = checkout.CombinedOutput()
		require.NoError(t, err, "git checkout failed: %s", out)

		apply := exec.Command("git", "apply", "--allow-empty", "-")
		apply.Dir = verify
		apply.Stdin = bytes.NewReader(patch)
		out, err = apply.CombinedOutput()
		require.NoError(t, err, "git apply failed: %s", out)

		requireFile := func(rel string, want []byte) {
			got, err := os.ReadFile(filepath.Join(verify, rel))
			require.NoError(t, err)
			require.Equal(t, want, got, "mismatch for %s", rel)
		}
		requireFile("base.txt", []byte("hello\ncommitted\nworking copy\n"))
		requireFile("added.txt", []byte("added\n"))
		requireFile("nested/more/x.txt", []byte("deeper\n"))
		requireFile("seed.bin", wantModifiedBinary)
		requireFile("fresh.bin", wantNewBinary)
		require.NoFileExists(t, filepath.Join(verify, "sub.txt"))
	})
}

func TestGeneratePatchFile(t *testing.T) {
	t.Run("writes a patch named for the base commit", func(t *testing.T) {
		eachLayout(t, "clone-local-work", func(t *testing.T, f fixture) {
			destDir := filepath.Join(f.tempDir, "patches")
			patchFile, err := f.client.GeneratePatchFile(destDir, nil)
			require.NoError(t, err)
			require.True(t, patchFile.Written)
			require.Equal(t, filepath.Join(destDir, f.baseSHA), patchFile.Path)

			contents, err := os.ReadFile(patchFile.Path)
			require.NoError(t, err)
			require.Contains(t, string(contents), "+working copy")
		})
	})

	t.Run("writes nothing when there are no changes", func(t *testing.T) {
		eachLayout(t, "clone", func(t *testing.T, f fixture) {
			patchFile, err := f.client.GeneratePatchFile(filepath.Join(f.tempDir, "patches"), nil)
			require.NoError(t, err)
			require.False(t, patchFile.Written)
			require.Empty(t, patchFile.Path)
		})
	})

	t.Run("reports LFS-tracked changes", func(t *testing.T) {
		eachLayout(t, "clone-lfs-file", func(t *testing.T, f fixture) {
			patchFile, err := f.client.GeneratePatchFile(filepath.Join(f.tempDir, "patches"), nil)
			require.NoError(t, err)
			require.False(t, patchFile.Written)
			require.Equal(t, 1, patchFile.LFSChangedFiles.Count)
		})
	})
}

func TestGenerateDirtyPatches(t *testing.T) {
	eachLayout(t, "clone-local-work", func(t *testing.T, f fixture) {
		patches, err := f.client.GenerateDirtyPatches()
		require.NoError(t, err)
		require.Empty(t, patches.Staged)
		require.Empty(t, patches.Unstaged)
		require.Nil(t, patches.Files)
		require.Nil(t, patches.NewFiles)
		require.Equal(t, 0, patches.Size())
	})
}

// A secondary workspace's .jj/repo is a file pointing at the primary one.
func TestSecondaryWorkspace(t *testing.T) {
	eachLayout(t, "clone-secondary-workspace", func(t *testing.T, f fixture) {
		workspace := filepath.Join(f.tempDir, "workspace")
		client := newClient(workspace)

		requireSamePath(t, workspace, client.GetTopLevel())

		require.NoError(t, os.WriteFile(filepath.Join(workspace, "added.txt"), []byte("from the workspace\n"), 0o644))

		sha, err := client.GetCommit()
		require.NoError(t, err)
		require.Equal(t, f.baseSHA, sha)

		patch, _, err := client.GeneratePatch(nil)
		require.NoError(t, err)
		require.Contains(t, string(patch), "b/added.txt")
		require.Contains(t, string(patch), "+from the workspace")
	})
}

func TestHasCommit(t *testing.T) {
	eachLayout(t, "clone-local-work", func(t *testing.T, f fixture) {
		head, err := f.client.GetHeadCommit()
		require.NoError(t, err)

		require.True(t, f.client.HasCommit(head))
		require.True(t, f.client.HasCommit(f.baseSHA))
		require.False(t, f.client.HasCommit("deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"))
		require.False(t, f.client.HasCommit(""))
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

func TestPushRef(t *testing.T) {
	t.Run("pushes the working copy commit", func(t *testing.T) {
		eachLayout(t, "clone-local-work", func(t *testing.T, f fixture) {
			head, err := f.client.GetHeadCommit()
			require.NoError(t, err)

			opts := vcstypes.PushRefOptions{Remote: f.origin, Refspec: "+" + head + ":refs/rwx/test"}
			require.NoError(t, f.client.PushRef(opts))

			require.Equal(t, head, originRef(t, f.origin))
		})
	})

	t.Run("validates its options", func(t *testing.T) {
		eachLayout(t, "clone", func(t *testing.T, f fixture) {
			require.ErrorContains(t, f.client.PushRef(vcstypes.PushRefOptions{Refspec: "a:b"}), "no remote provided")
			require.ErrorContains(t, f.client.PushRef(vcstypes.PushRefOptions{Remote: f.origin}), "no refspec provided")
		})
	})

	t.Run("rejects a commit missing from the store", func(t *testing.T) {
		eachLayout(t, "clone", func(t *testing.T, f fixture) {
			missing := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

			err := f.client.PushRef(vcstypes.PushRefOptions{
				Remote:  f.origin,
				Refspec: "+" + missing + ":refs/rwx/test",
			})
			require.ErrorContains(t, err, "not in the git store")
			require.ErrorContains(t, err, missing)
		})
	})

	t.Run("rejects a conflicted working copy", func(t *testing.T) {
		eachLayout(t, "clone-conflicted-merge", func(t *testing.T, f fixture) {
			head, err := f.client.GetHeadCommit()
			require.NoError(t, err)

			body, err := os.ReadFile(filepath.Join(f.root, "base.txt"))
			require.NoError(t, err)
			require.Contains(t, string(body), "<<<<<<<", "fixture should leave the working copy conflicted")

			err = f.client.PushRef(vcstypes.PushRefOptions{Remote: f.origin, Refspec: "+" + head + ":refs/rwx/test"})
			require.ErrorContains(t, err, "unresolved conflicts")
			require.ErrorContains(t, err, "jj resolve")

			require.Empty(t, originRef(t, f.origin), "nothing should have been pushed")
		})
	})
}

// originRef resolves the ref PushRef targets, or "" when it was never created.
func originRef(t *testing.T, origin string) string {
	t.Helper()

	out, err := exec.Command("git", "-C", origin, "rev-parse", "refs/rwx/test").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
