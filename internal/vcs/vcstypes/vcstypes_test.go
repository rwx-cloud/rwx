package vcstypes_test

import (
	"fmt"
	"testing"

	"github.com/rwx-cloud/rwx/internal/vcs/vcstypes"
	"github.com/stretchr/testify/require"
)

func TestPatchErrorReason(t *testing.T) {
	cases := []struct {
		stderr string
		want   string
	}{
		{"fatal: bad object 9a3b1c4e", "shallow_clone"},
		{"fatal: pathspec '.rwx' is beyond a symbolic link", "beyond_symlink"},
		{"error: external filter 'git-lfs filter-process' failed", "missing_external_filter"},
		{"signal: killed", "oom_killed"},
		{"error: patch failed: main.go:12", "patch_conflict"},
		{"error: foo.txt: patch does not apply", "patch_conflict"},
		{"error: bar.txt: already exists in working directory", "already_exists"},
		{"error: corrupt patch at line 3", "corrupt_patch"},
		{"fatal: something else entirely", "unknown"},
		{"", "unknown"},
	}

	for _, tc := range cases {
		pe := &vcstypes.PatchError{Stderr: tc.stderr}
		require.Equal(t, tc.want, pe.Reason(), "stderr: %q", tc.stderr)
	}
}

func TestPatchFailureReason(t *testing.T) {
	t.Run("nil is empty", func(t *testing.T) {
		require.Equal(t, "", vcstypes.PatchFailureReason(nil))
	})

	t.Run("prefers a wrapped *PatchError's structured stderr", func(t *testing.T) {
		err := fmt.Errorf("failed to generate dirty patch: %w", &vcstypes.PatchError{Stderr: "fatal: bad object deadbeef"})
		require.Equal(t, "shallow_clone", vcstypes.PatchFailureReason(err))
	})

	cases := []struct {
		name string
		msg  string
		want string
	}{
		{"git apply conflict", "failed to sync changes to sandbox: git apply failed: error: patch failed: a.go:1", "patch_conflict"},
		{"already exists", "failed to sync changes to sandbox: git apply failed: error: b.go: already exists in working directory", "already_exists"},
		{"corrupt patch", "failed to sync changes to sandbox: git apply failed: error: corrupt patch at line 9", "corrupt_patch"},
		{"lfs changed", "3 LFS file(s) changed locally and cannot be synced to the sandbox", "lfs_changed"},
		{"unclassified", "failed to apply patch on sandbox: connection reset", "unknown"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, vcstypes.PatchFailureReason(fmt.Errorf("%s", tc.msg)))
		})
	}
}

func TestCommitMismatchNote(t *testing.T) {
	t.Run("returns note with short SHAs when commits differ", func(t *testing.T) {
		note := vcstypes.CommitMismatchNote(
			"aaaaaaa1111111222222233333334444444",
			"bbbbbbb5555555666666677777778888888",
		)
		require.Equal(t, "Note: you're currently on commit aaaaaaa but the most recent run on this branch was for commit bbbbbbb", note)
	})

	t.Run("returns empty when commits match exactly", func(t *testing.T) {
		note := vcstypes.CommitMismatchNote(
			"abc123def456",
			"abc123def456",
		)
		require.Equal(t, "", note)
	})

	t.Run("returns empty when head is a prefix of run commit", func(t *testing.T) {
		note := vcstypes.CommitMismatchNote(
			"abc123d",
			"abc123def456789",
		)
		require.Equal(t, "", note)
	})

	t.Run("returns empty when run commit is a prefix of head", func(t *testing.T) {
		note := vcstypes.CommitMismatchNote(
			"abc123def456789",
			"abc123d",
		)
		require.Equal(t, "", note)
	})

	t.Run("preserves short SHAs when already short", func(t *testing.T) {
		note := vcstypes.CommitMismatchNote("abc", "def")
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
			require.Equal(t, tt.expected, vcstypes.RepoNameFromOriginUrl(tt.input))
		})
	}
}
