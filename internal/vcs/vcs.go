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

// New picks the backend for dir. RWX_VCS forces one; otherwise the innermost
// repository wins, with jj taking the tie in a colocated repo. Detection reads
// the filesystem rather than spawning either tool, since most commands never
// touch version control.
func New(dir string) Client {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("RWX_VCS"))) {
	case "git":
		return newGit(dir)
	case "jj":
		return newJJ(dir)
	}

	if !jjAvailable() {
		return newGit(dir)
	}

	jjRoot := nearestRoot(dir, ".jj")
	if jjRoot == "" {
		return newGit(dir)
	}

	gitRoot := nearestRoot(dir, ".git")
	if gitRoot != "" && isProperSubpath(jjRoot, gitRoot) {
		return newGit(dir)
	}

	return newJJ(dir)
}

// nearestRoot walks up from dir to the closest directory holding marker, or ""
// when none does. .git may be a file in a linked work tree, so any entry counts.
func nearestRoot(dir, marker string) string {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}

	for {
		if _, err := os.Lstat(filepath.Join(dir, marker)); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
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
