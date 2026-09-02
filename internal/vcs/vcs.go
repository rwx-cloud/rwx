package vcs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rwx-cloud/rwx/internal/git"
	"github.com/rwx-cloud/rwx/internal/jj"
	"github.com/rwx-cloud/rwx/internal/vcs/vcstypes"
)

type (
	UntrackedFilesMetadata  = vcstypes.UntrackedFilesMetadata
	LFSChangedFilesMetadata = vcstypes.LFSChangedFilesMetadata
	PatchFile               = vcstypes.PatchFile
	DirtyPatches            = vcstypes.DirtyPatches
	PushRefOptions          = vcstypes.PushRefOptions
	PatchError              = vcstypes.PatchError
)

var (
	CommitMismatchNote    = vcstypes.CommitMismatchNote
	RepoNameFromOriginUrl = vcstypes.RepoNameFromOriginUrl
	PatchFailureReason    = vcstypes.PatchFailureReason
)

type Client interface {
	// MissingDependency names the executable the backend needs but cannot
	// find, or "" when everything is present.
	MissingDependency() string
	IsInsideWorkTree() bool
	GetTopLevel() string
	// GetBranch returns the branch naming the current position, or "" when
	// there is none (a detached HEAD under git, no bookmark at or below @ under
	// jj).
	GetBranch() string
	GetHeadCommit() (string, error)
	// GetShortHead returns a short identifier for the current position that
	// stays stable while the working copy is edited. It keys sandbox sessions
	// when GetBranch is empty.
	GetShortHead() string
	GetCommit() (string, error)
	// GetLastCommit returns the commit a run made from this position would be
	// for. It differs from GetHeadCommit only when the working copy is itself a
	// commit, as under jj, where an empty @ is skipped.
	GetLastCommit() (string, error)
	GetOriginUrl() string
	GeneratePatchFile(destDir string, pathspec []string) (PatchFile, error)
	GeneratePatch(pathspec []string) ([]byte, *LFSChangedFilesMetadata, error)
	GenerateDirtyPatches() (DirtyPatches, error)
	IsAncestor(candidateSHA, headRef string) bool
	PushRef(opts PushRefOptions) error
	ApplyPatch(patch []byte) *exec.Cmd
	ApplyPatchReject(patch []byte) *exec.Cmd
}

var (
	_ Client = (*git.Client)(nil)
	_ Client = (*jj.Client)(nil)
)

const (
	gitBinary = "git"
	jjBinary  = "jj"
)

// New picks the backend for dir. The innermost repository containing dir wins,
// so a jj repo nested inside an unrelated git checkout is not mistaken for part
// of it. A colocated jj repo has both roots at the same path, and jj takes that
// tie.
func New(dir string) Client {
	gitClient := newGit(dir)
	if !jjAvailable() {
		return gitClient
	}

	jjClient := newJJ(dir)
	jjRoot := jjClient.GetTopLevel()
	if jjRoot == "" {
		return gitClient
	}

	// A git repository nested inside the jj workspace is its own repository.
	gitRoot := gitClient.GetTopLevel()
	if gitRoot != "" && isProperSubpath(jjRoot, gitRoot) {
		return gitClient
	}

	return jjClient
}

// isProperSubpath reports whether child sits strictly below parent.
func isProperSubpath(parent, child string) bool {
	if parent == "" || child == "" {
		return false
	}

	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}

	return rel != "." && !strings.HasPrefix(rel, "..")
}

func newGit(dir string) *git.Client {
	return &git.Client{Binary: gitBinary, Dir: dir}
}

func newJJ(dir string) *jj.Client {
	return &jj.Client{Binary: jjBinary, GitBinary: gitBinary, Dir: dir, Stderr: os.Stderr}
}

func jjAvailable() bool {
	_, err := exec.LookPath(jjBinary)
	return err == nil
}
