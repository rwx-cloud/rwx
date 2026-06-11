package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/git"
	"github.com/stretchr/testify/require"
)

func repoFixture(t *testing.T, fixturePath string) (string, string) {
	t.Helper()

	tempDir := t.TempDir()

	fixtureInfo, err := os.Stat(fixturePath)
	if err != nil {
		t.Fatalf("could not find fixture: %v", err)
	}
	// Cache clear if the fixture file changes
	_ = fixtureInfo.ModTime()

	base := filepath.Base(fixturePath)

	cmd := exec.Command("cp", fixturePath, tempDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to copy fixture %s to tempDir %s: %v", fixturePath, tempDir, err)
	}

	cmd = exec.Command("bash", base)
	cmd.Dir = tempDir

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to initialize fixture %s: %v\nOutput:%s", fixturePath, err, out)
	}

	return tempDir, strings.TrimSpace(string(out))
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestGetBranch(t *testing.T) {
	t.Run("returns empty if git is not installed", func(t *testing.T) {
		client := &git.Client{Binary: "fake", Dir: ""}
		branch := client.GetBranch()
		require.Equal(t, "", branch)
	})

	t.Run("returns empty if we're not in a git repo", func(t *testing.T) {
		tempDir := t.TempDir()

		client := &git.Client{Binary: "git", Dir: tempDir}
		branch := client.GetBranch()
		require.Equal(t, "", branch)
	})

	t.Run("returns empty if we're in detached HEAD state", func(t *testing.T) {
		repo, _ := repoFixture(t, "testdata/GetBranch-detached-head")

		client := &git.Client{Binary: "git", Dir: repo}
		branch := client.GetBranch()
		require.Equal(t, "", branch)
	})

	t.Run("returns a branch if we're on a branch", func(t *testing.T) {
		repo, expected := repoFixture(t, "testdata/GetBranch-branch")

		client := &git.Client{Binary: "git", Dir: repo}
		branch := client.GetBranch()
		require.Equal(t, expected, branch)
	})
}

func TestGetHeadCommit(t *testing.T) {
	t.Run("returns HEAD for a repo with commits", func(t *testing.T) {
		repo, _ := repoFixture(t, "testdata/GetCommit-no-remote")
		expected := mustGit(t, repo, "rev-parse", "HEAD")

		client := &git.Client{Binary: "git", Dir: repo}
		sha, err := client.GetHeadCommit()

		require.NoError(t, err)
		require.Equal(t, expected, sha)
	})

	t.Run("returns error outside a git repo", func(t *testing.T) {
		client := &git.Client{Binary: "git", Dir: t.TempDir()}

		sha, err := client.GetHeadCommit()

		require.Equal(t, "", sha)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unable to resolve HEAD")
	})

	t.Run("returns error for an unborn branch", func(t *testing.T) {
		repo, _ := repoFixture(t, "testdata/GetCommit-no-commits")

		client := &git.Client{Binary: "git", Dir: repo}
		sha, err := client.GetHeadCommit()

		require.Equal(t, "", sha)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unable to resolve HEAD")
	})
}

func TestGetCommit(t *testing.T) {
	t.Run("returns empty with no error if git is not installed", func(t *testing.T) {
		client := &git.Client{Binary: "fake", Dir: ""}
		sha, err := client.GetCommit()
		require.NoError(t, err)
		require.Equal(t, "", sha)
	})

	t.Run("returns empty with no error if we're not in a git repo", func(t *testing.T) {
		tempDir := t.TempDir()

		client := &git.Client{Binary: "git", Dir: tempDir}
		sha, err := client.GetCommit()
		require.NoError(t, err)
		require.Equal(t, "", sha)
	})

	t.Run("returns error if remote is not set", func(t *testing.T) {
		repo, _ := repoFixture(t, "testdata/GetCommit-no-remote")

		client := &git.Client{Binary: "git", Dir: repo}
		sha, err := client.GetCommit()
		require.Equal(t, "", sha)
		require.EqualError(t, err, "no git remote named 'origin' is configured (set RWX_GIT_REMOTE to use a different remote)")
	})

	t.Run("returns error if branch has no commits", func(t *testing.T) {
		repo, _ := repoFixture(t, "testdata/GetCommit-no-commits")

		client := &git.Client{Binary: "git", Dir: repo}
		sha, err := client.GetCommit()
		require.Equal(t, "", sha)
		require.EqualError(t, err, "current branch has no commits")
	})

	t.Run("returns error if remote origin is not set", func(t *testing.T) {
		repo, _ := repoFixture(t, "testdata/GetCommit-no-remote-origin")

		client := &git.Client{Binary: "git", Dir: filepath.Join(repo, "repo")}
		sha, err := client.GetCommit()
		require.Equal(t, "", sha)
		require.EqualError(t, err, "no git remote named 'origin' is configured (set RWX_GIT_REMOTE to use a different remote)")
	})

	t.Run("returns error if there is no common ancestor (orphan branch)", func(t *testing.T) {
		repo, _ := repoFixture(t, "testdata/GetCommit-no-common-ancestor")

		client := &git.Client{Binary: "git", Dir: filepath.Join(repo, "repo")}
		sha, err := client.GetCommit()
		require.Equal(t, "", sha)
		require.EqualError(t, err, "current branch has no commits in common with the 'origin' remote (set RWX_GIT_REMOTE to use a different remote)")
	})

	t.Run("detached HEAD state", func(t *testing.T) {
		t.Run("at origin", func(t *testing.T) {
			repo, expected := repoFixture(t, "testdata/GetCommit-detached-head")

			client := &git.Client{Binary: "git", Dir: filepath.Join(repo, "repo")}
			commit, err := client.GetCommit()
			require.NoError(t, err)
			require.Equal(t, expected, commit)
		})

		t.Run("diverged from origin", func(t *testing.T) {
			repo, expected := repoFixture(t, "testdata/GetCommit-detached-head-diverged")

			client := &git.Client{Binary: "git", Dir: filepath.Join(repo, "repo")}
			commit, err := client.GetCommit()
			require.NoError(t, err)
			require.Equal(t, expected, commit)
		})
	})

	t.Run("when we have a branch checked out", func(t *testing.T) {
		t.Run("main at origin", func(t *testing.T) {
			repo, expected := repoFixture(t, "testdata/GetCommit-main-at-origin")

			client := &git.Client{Binary: "git", Dir: filepath.Join(repo, "repo")}
			commit, err := client.GetCommit()
			require.NoError(t, err)
			require.Equal(t, expected, commit)
		})

		t.Run("main behind origin", func(t *testing.T) {
			repo, expected := repoFixture(t, "testdata/GetCommit-main-behind-origin")

			client := &git.Client{Binary: "git", Dir: filepath.Join(repo, "repo")}
			commit, err := client.GetCommit()
			require.NoError(t, err)
			require.Equal(t, expected, commit)
		})

		t.Run("main ahead of origin", func(t *testing.T) {
			repo, expected := repoFixture(t, "testdata/GetCommit-main-ahead-of-origin")

			client := &git.Client{Binary: "git", Dir: filepath.Join(repo, "repo")}
			commit, err := client.GetCommit()
			require.NoError(t, err)
			require.Equal(t, expected, commit)
		})

		t.Run("feature from local", func(t *testing.T) {
			repo, expected := repoFixture(t, "testdata/GetCommit-feature-from-local")

			client := &git.Client{Binary: "git", Dir: filepath.Join(repo, "repo")}
			commit, err := client.GetCommit()
			require.NoError(t, err)
			require.Equal(t, expected, commit)
		})

		t.Run("feature from feature", func(t *testing.T) {
			repo, expected := repoFixture(t, "testdata/GetCommit-feature-from-feature")

			client := &git.Client{Binary: "git", Dir: filepath.Join(repo, "repo")}
			commit, err := client.GetCommit()
			require.NoError(t, err)
			require.Equal(t, expected, commit)
		})

		t.Run("feature from main origin moved", func(t *testing.T) {
			repo, expected := repoFixture(t, "testdata/GetCommit-feature-from-main-origin-moved")

			client := &git.Client{Binary: "git", Dir: filepath.Join(repo, "repo")}
			commit, err := client.GetCommit()
			require.NoError(t, err)
			require.Equal(t, expected, commit)
		})

		t.Run("feature from feature origin moved", func(t *testing.T) {
			repo, expected := repoFixture(t, "testdata/GetCommit-feature-from-feature-origin-moved")

			client := &git.Client{Binary: "git", Dir: filepath.Join(repo, "repo")}
			commit, err := client.GetCommit()
			require.NoError(t, err)
			require.Equal(t, expected, commit)
		})
	})
}

func TestGetOriginUrl(t *testing.T) {
	t.Run("returns empty if git is not installed", func(t *testing.T) {
		client := &git.Client{Binary: "fake", Dir: ""}
		url := client.GetOriginUrl()
		require.Equal(t, "", url)
	})

	t.Run("returns empty if we're not in a git repo", func(t *testing.T) {
		tempDir := t.TempDir()

		client := &git.Client{Binary: "git", Dir: tempDir}
		url := client.GetOriginUrl()

		require.Equal(t, "", url)
	})

	t.Run("returns empty if there are no remotes", func(t *testing.T) {
		repo, _ := repoFixture(t, "testdata/GetOriginUrl-no-remote")

		client := &git.Client{Binary: "git", Dir: filepath.Join(repo, "repo")}
		url := client.GetOriginUrl()

		require.Equal(t, "", url)
	})

	t.Run("returns empty if there is no remote origin", func(t *testing.T) {
		repo, _ := repoFixture(t, "testdata/GetOriginUrl-no-remote-origin")

		client := &git.Client{Binary: "git", Dir: filepath.Join(repo, "repo")}
		url := client.GetOriginUrl()

		require.Equal(t, "", url)
	})

	t.Run("returns origin url even if there are many remotes", func(t *testing.T) {
		repo, expected := repoFixture(t, "testdata/GetOriginUrl-many-remotes")

		client := &git.Client{Binary: "git", Dir: filepath.Join(repo, "repo")}
		url := client.GetOriginUrl()

		require.NotEqual(t, "", expected)
		require.Equal(t, expected, url)
	})

	t.Run("returns origin url", func(t *testing.T) {
		repo, expected := repoFixture(t, "testdata/GetOriginUrl")

		client := &git.Client{Binary: "git", Dir: filepath.Join(repo, "repo")}
		url := client.GetOriginUrl()

		require.NotEqual(t, "", expected)
		require.Equal(t, expected, url)
	})
}

func TestGeneratePatch(t *testing.T) {
	t.Run("returns nil when git is not installed", func(t *testing.T) {
		client := &git.Client{Binary: "fake", Dir: ""}
		patch, lfs, err := client.GeneratePatch(nil)

		require.NoError(t, err)
		require.Nil(t, patch)
		require.Nil(t, lfs)
	})

	t.Run("returns nil when we can't determine a base commit", func(t *testing.T) {
		client := &git.Client{Binary: "git", Dir: "/tmp"}
		patch, lfs, err := client.GeneratePatch(nil)

		require.NoError(t, err)
		require.Nil(t, patch)
		require.Nil(t, lfs)
	})

	t.Run("returns nil patch when there is no diff", func(t *testing.T) {
		tempDir, _ := repoFixture(t, "testdata/GeneratePatchFile-no-diff")

		client := &git.Client{Binary: "git", Dir: filepath.Join(tempDir, "repo")}
		patch, lfs, err := client.GeneratePatch(nil)

		require.NoError(t, err)
		require.Nil(t, patch)
		require.Nil(t, lfs)
	})

	t.Run("returns patch bytes when there's a diff", func(t *testing.T) {
		tempDir, _ := repoFixture(t, "testdata/GeneratePatchFile-diff")

		client := &git.Client{Binary: "git", Dir: filepath.Join(tempDir, "repo")}
		patch, lfs, err := client.GeneratePatch(nil)

		require.NoError(t, err)
		require.NotNil(t, patch)
		require.Contains(t, string(patch), "new file mode 100644")
		require.Nil(t, lfs)
	})

	t.Run("returns patch bytes for unstaged changes", func(t *testing.T) {
		// This fixture has unstaged binary changes (not added)
		tempDir, _ := repoFixture(t, "testdata/GeneratePatchFile-diff-binary")

		client := &git.Client{Binary: "git", Dir: filepath.Join(tempDir, "repo")}
		patch, lfs, err := client.GeneratePatch(nil)

		require.NoError(t, err)
		require.NotNil(t, patch) // Now includes unstaged changes
		require.Nil(t, lfs)
	})
}

func TestGenerateDirtyPatches(t *testing.T) {
	repo := t.TempDir()
	mustGit(t, repo, "init")
	mustGit(t, repo, "config", "user.email", "test@example.com")
	mustGit(t, repo, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\n"), 0o644))
	mustGit(t, repo, "add", "tracked.txt")
	mustGit(t, repo, "commit", "-m", "base")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "staged.txt"), []byte("staged\n"), 0o644))
	mustGit(t, repo, "add", "staged.txt")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\nunstaged\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("untracked\n"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(repo, "dir with space"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "dir with space", "quote'file.txt"), []byte("quoted\n"), 0o644))

	client := &git.Client{Binary: "git", Dir: repo}
	patches, err := client.GenerateDirtyPatches()
	require.NoError(t, err)

	require.Contains(t, string(patches.Staged), "staged.txt")
	require.NotContains(t, string(patches.Staged), "untracked.txt")
	require.Contains(t, string(patches.Unstaged), "tracked.txt")
	require.Contains(t, string(patches.Unstaged), "untracked.txt")
	require.ElementsMatch(t, []string{"staged.txt", "tracked.txt", "untracked.txt", "dir with space/quote'file.txt"}, patches.Files)
	require.ElementsMatch(t, []string{"staged.txt", "untracked.txt", "dir with space/quote'file.txt"}, patches.NewFiles)

	cachedNames := mustGit(t, repo, "diff", "--cached", "--name-only")
	require.Equal(t, "staged.txt", cachedNames)
}

func TestPushRef(t *testing.T) {
	source := t.TempDir()
	mustGit(t, source, "init")
	mustGit(t, source, "config", "user.email", "test@example.com")
	mustGit(t, source, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(source, "pushed.txt"), []byte("pushed\n"), 0o644))
	mustGit(t, source, "add", "pushed.txt")
	mustGit(t, source, "commit", "-m", "pushed")
	head := mustGit(t, source, "rev-parse", "HEAD")

	target := t.TempDir()
	mustGit(t, target, "init", "--bare")

	client := &git.Client{Binary: "git", Dir: source}
	err := client.PushRef(git.PushRefOptions{
		Remote:  target,
		Refspec: head + ":refs/rwx/push/test",
		Env:     []string{"RWX_TEST_PUSH_ENV=1"},
	})

	require.NoError(t, err)
	pushed := mustGit(t, target, "rev-parse", "refs/rwx/push/test")
	require.Equal(t, head, pushed)
}

func TestGeneratePatchFile(t *testing.T) {
	t.Run("does not write a patch file", func(t *testing.T) {
		t.Run("when git is not installed", func(t *testing.T) {
			client := &git.Client{Binary: "fake", Dir: ""}
			patchFile, err := client.GeneratePatchFile("", nil)

			require.Error(t, err)
			require.Equal(t, false, patchFile.Written)
		})

		t.Run("when we can't determine a diff", func(t *testing.T) {
			client := &git.Client{Binary: "git", Dir: "/tmp"}
			patchFile, err := client.GeneratePatchFile("", nil)

			require.Error(t, err)
			require.Equal(t, false, patchFile.Written)
		})

		t.Run("when there is no diff", func(t *testing.T) {
			tempDir, _ := repoFixture(t, "testdata/GeneratePatchFile-no-diff")

			client := &git.Client{Binary: "git", Dir: filepath.Join(tempDir, "repo")}
			patchFile, err := client.GeneratePatchFile(client.Dir, nil)

			require.NoError(t, err)
			require.Equal(t, false, patchFile.Written)
		})

		t.Run("when there are uncommitted changes to LFS tracked files", func(t *testing.T) {
			tempDir, expected := repoFixture(t, "testdata/GeneratePatchFile-lfs")

			client := &git.Client{Binary: "git", Dir: filepath.Join(tempDir, "repo")}
			patchFile, err := client.GeneratePatchFile(client.Dir, nil)

			require.NoError(t, err)
			require.Equal(t, false, patchFile.Written)
			require.ElementsMatch(t, strings.Split(expected, " "), patchFile.LFSChangedFiles.Files)
			require.Equal(t, 2, patchFile.LFSChangedFiles.Count)
		})
	})

	t.Run("writes a patch file", func(t *testing.T) {
		t.Run("when there's an uncommitted diff", func(t *testing.T) {
			tempDir, sha := repoFixture(t, "testdata/GeneratePatchFile-diff")

			client := &git.Client{Binary: "git", Dir: filepath.Join(tempDir, "repo")}
			patchFile, err := client.GeneratePatchFile(client.Dir, nil)
			require.NoError(t, err)

			require.Equal(t, true, patchFile.Written)
			require.Equal(t, filepath.Join(client.Dir, sha), patchFile.Path)

			patch, err := os.ReadFile(patchFile.Path)
			require.NoError(t, err)
			require.Contains(t, string(patch), "new file mode 100644")
		})

		t.Run("when there's a committed diff", func(t *testing.T) {
			tempDir, sha := repoFixture(t, "testdata/GeneratePatchFile-diff-committed")

			client := &git.Client{Binary: "git", Dir: filepath.Join(tempDir, "repo")}
			patchFile, err := client.GeneratePatchFile(client.Dir, nil)
			require.NoError(t, err)

			require.Equal(t, true, patchFile.Written)
			require.Equal(t, filepath.Join(client.Dir, sha), patchFile.Path)

			patch, err := os.ReadFile(patchFile.Path)
			require.NoError(t, err)
			require.Contains(t, string(patch), "new file mode 100644")

			require.Equal(t, []string{}, patchFile.UntrackedFiles.Files)
			require.Equal(t, 0, patchFile.UntrackedFiles.Count)
		})

		t.Run("including changes to binary files", func(t *testing.T) {
			tempDir, sha := repoFixture(t, "testdata/GeneratePatchFile-diff-binary")

			client := &git.Client{Binary: "git", Dir: filepath.Join(tempDir, "repo")}
			patchFile, err := client.GeneratePatchFile(client.Dir, nil)
			require.NoError(t, err)

			require.Equal(t, true, patchFile.Written)
			require.Equal(t, filepath.Join(client.Dir, sha), patchFile.Path)

			patch, err := os.ReadFile(patchFile.Path)
			require.NoError(t, err)
			require.Contains(t, string(patch), "GIT binary patch")

			require.Equal(t, []string{}, patchFile.UntrackedFiles.Files)
			require.Equal(t, 0, patchFile.UntrackedFiles.Count)
		})

		t.Run("including untracked files with other changes", func(t *testing.T) {
			tempDir, sha := repoFixture(t, "testdata/GeneratePatchFile-diff-untracked")

			client := &git.Client{Binary: "git", Dir: filepath.Join(tempDir, "repo")}
			patchFile, err := client.GeneratePatchFile(client.Dir, nil)
			require.NoError(t, err)

			require.Equal(t, true, patchFile.Written)
			require.Equal(t, filepath.Join(client.Dir, sha), patchFile.Path)

			patch, err := os.ReadFile(patchFile.Path)
			require.NoError(t, err)
			require.Contains(t, string(patch), "new file mode 100644")
			require.Contains(t, string(patch), "bar.txt")

			require.Equal(t, []string{}, patchFile.UntrackedFiles.Files)
			require.Equal(t, 0, patchFile.UntrackedFiles.Count)
			require.Equal(t, "foo.txt", mustGit(t, client.Dir, "diff", "--cached", "--name-only"))
			require.Contains(t, strings.Split(mustGit(t, client.Dir, "ls-files", "--others", "--exclude-standard"), "\n"), "bar.txt")
		})

		t.Run("with only untracked files", func(t *testing.T) {
			tempDir, sha := repoFixture(t, "testdata/GeneratePatchFile-diff-untracked-only")

			client := &git.Client{Binary: "git", Dir: filepath.Join(tempDir, "repo")}
			patchFile, err := client.GeneratePatchFile(client.Dir, nil)
			require.NoError(t, err)

			require.Equal(t, true, patchFile.Written)
			require.Equal(t, filepath.Join(client.Dir, sha), patchFile.Path)

			patch, err := os.ReadFile(patchFile.Path)
			require.NoError(t, err)
			require.Contains(t, string(patch), "new file mode 100644")
			require.Contains(t, string(patch), "untracked.txt")

			require.Equal(t, []string{}, patchFile.UntrackedFiles.Files)
			require.Equal(t, 0, patchFile.UntrackedFiles.Count)
			require.Equal(t, "", mustGit(t, client.Dir, "diff", "--cached", "--name-only"))
			require.Contains(t, strings.Split(mustGit(t, client.Dir, "ls-files", "--others", "--exclude-standard"), "\n"), "untracked.txt")
		})

		t.Run("excluding paths via pathspec", func(t *testing.T) {
			tempDir, sha := repoFixture(t, "testdata/GeneratePatchFile-diff-exclude")

			client := &git.Client{Binary: "git", Dir: filepath.Join(tempDir, "repo")}
			patchFile, err := client.GeneratePatchFile(client.Dir, []string{".", ":!.rwx"})
			require.NoError(t, err)

			require.Equal(t, true, patchFile.Written)
			require.Equal(t, filepath.Join(client.Dir, sha), patchFile.Path)

			patch, err := os.ReadFile(patchFile.Path)
			require.NoError(t, err)
			require.Contains(t, string(patch), "included.txt")
			require.Contains(t, string(patch), "untracked.txt")
			require.NotContains(t, string(patch), "excluded.txt")
			require.NotContains(t, string(patch), "untracked-excluded.txt")

			require.Equal(t, []string{}, patchFile.UntrackedFiles.Files)
			require.Equal(t, 0, patchFile.UntrackedFiles.Count)
		})

		t.Run("returns a PatchError identifying the failing git command", func(t *testing.T) {
			tempDir, _ := repoFixture(t, "testdata/GeneratePatchFile-diff")
			client := &git.Client{Binary: "git", Dir: filepath.Join(tempDir, "repo")}

			// An invalid pathspec magic makes `git diff --name-only` fail
			// deterministically, exercising the error-capture path.
			_, err := client.GeneratePatchFile(t.TempDir(), []string{":(top,bogusmagic)x"})
			require.Error(t, err)

			var pe *git.PatchError
			require.ErrorAs(t, err, &pe)
			require.Equal(t, "diff_name_only", pe.Command)
			require.Equal(t, 128, pe.ExitCode)
			require.Contains(t, pe.Stderr, "Invalid pathspec magic")
			// The underlying git stderr is surfaced in the error message.
			require.Contains(t, pe.Error(), "failed to generate patch (git diff --name-only):")
		})
	})
}

func TestPatchErrorReason(t *testing.T) {
	cases := []struct {
		stderr string
		want   string
	}{
		{"fatal: bad object 9a3b1c4e", "shallow_clone"},
		{"fatal: pathspec '.rwx' is beyond a symbolic link", "beyond_symlink"},
		{"error: external filter 'git-lfs filter-process' failed", "missing_external_filter"},
		{"signal: killed", "oom_killed"},
		{"fatal: something else entirely", "unknown"},
		{"", "unknown"},
	}

	for _, tc := range cases {
		pe := &git.PatchError{Stderr: tc.stderr}
		require.Equal(t, tc.want, pe.Reason(), "stderr: %q", tc.stderr)
	}
}

func TestIsAncestor(t *testing.T) {
	t.Run("returns true when candidate is ancestor of HEAD", func(t *testing.T) {
		repo, firstSHA := repoFixture(t, "testdata/IsAncestor-linear")
		client := &git.Client{Binary: "git", Dir: repo}
		require.True(t, client.IsAncestor(firstSHA, "HEAD"))
	})

	t.Run("returns false when candidate is descendant of HEAD", func(t *testing.T) {
		repo, firstSHA := repoFixture(t, "testdata/IsAncestor-linear")
		client := &git.Client{Binary: "git", Dir: repo}
		headSHA := client.GetHead()
		require.False(t, client.IsAncestor(headSHA, firstSHA))
	})

	t.Run("returns false for unrelated SHA", func(t *testing.T) {
		repo, _ := repoFixture(t, "testdata/IsAncestor-linear")
		client := &git.Client{Binary: "git", Dir: repo}
		require.False(t, client.IsAncestor("deadbeefdeadbeef", "HEAD"))
	})

	t.Run("returns true when candidate equals HEAD", func(t *testing.T) {
		repo, _ := repoFixture(t, "testdata/IsAncestor-linear")
		client := &git.Client{Binary: "git", Dir: repo}
		require.True(t, client.IsAncestor("HEAD", "HEAD"))
	})

	t.Run("returns false when not in a git repo", func(t *testing.T) {
		client := &git.Client{Binary: "git", Dir: t.TempDir()}
		require.False(t, client.IsAncestor("abc", "HEAD"))
	})
}

func TestApplyPatch(t *testing.T) {
	t.Run("applies root-relative patches from a subdirectory", func(t *testing.T) {
		repo := t.TempDir()
		var err error
		repo, err = filepath.EvalSymlinks(repo)
		require.NoError(t, err)
		mustGit(t, repo, "init")
		mustGit(t, repo, "config", "user.email", "test@example.com")
		mustGit(t, repo, "config", "user.name", "Test")

		rootFile := filepath.Join(repo, "root.txt")
		subdir := filepath.Join(repo, "subdir")
		require.NoError(t, os.MkdirAll(subdir, 0o755))
		require.NoError(t, os.WriteFile(rootFile, []byte("old\n"), 0o644))
		mustGit(t, repo, "add", "root.txt")
		mustGit(t, repo, "commit", "-m", "initial")

		require.NoError(t, os.WriteFile(rootFile, []byte("new\n"), 0o644))
		diffCmd := exec.Command("git", "diff", "--binary")
		diffCmd.Dir = repo
		patch, err := diffCmd.Output()
		require.NoError(t, err)
		mustGit(t, repo, "reset", "--hard", "HEAD")

		client := &git.Client{Binary: "git", Dir: subdir}
		cmd := client.ApplyPatch(patch)
		require.Equal(t, repo, cmd.Dir)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))

		content, err := os.ReadFile(rootFile)
		require.NoError(t, err)
		require.Equal(t, "new\n", string(content))
		require.Equal(t, repo, client.ApplyPatchReject(patch).Dir)
	})
}

func TestCommitMismatchNote(t *testing.T) {
	t.Run("returns note with short SHAs when commits differ", func(t *testing.T) {
		note := git.CommitMismatchNote(
			"aaaaaaa1111111222222233333334444444",
			"bbbbbbb5555555666666677777778888888",
		)
		require.Equal(t, "Note: you're currently on commit aaaaaaa but the most recent run on this branch was for commit bbbbbbb", note)
	})

	t.Run("returns empty when commits match exactly", func(t *testing.T) {
		note := git.CommitMismatchNote(
			"abc123def456",
			"abc123def456",
		)
		require.Equal(t, "", note)
	})

	t.Run("returns empty when head is a prefix of run commit", func(t *testing.T) {
		note := git.CommitMismatchNote(
			"abc123d",
			"abc123def456789",
		)
		require.Equal(t, "", note)
	})

	t.Run("returns empty when run commit is a prefix of head", func(t *testing.T) {
		note := git.CommitMismatchNote(
			"abc123def456789",
			"abc123d",
		)
		require.Equal(t, "", note)
	})

	t.Run("preserves short SHAs when already short", func(t *testing.T) {
		note := git.CommitMismatchNote("abc", "def")
		require.Equal(t, "Note: you're currently on commit abc but the most recent run on this branch was for commit def", note)
	})
}

func TestRepoNameFromOriginUrl(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"SSH URL", "git@github.com:rwx-cloud/rwx.git", "rwx"},
		{"HTTPS URL", "https://github.com/rwx-cloud/rwx.git", "rwx"},
		{"SSH URL without .git suffix", "git@github.com:rwx-cloud/rwx", "rwx"},
		{"HTTPS URL without .git suffix", "https://github.com/rwx-cloud/rwx", "rwx"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, git.RepoNameFromOriginUrl(tt.input))
		})
	}
}
