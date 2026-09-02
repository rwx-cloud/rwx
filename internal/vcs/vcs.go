package vcs

import (
	"os/exec"

	"github.com/rwx-cloud/rwx/internal/git"
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
	// there is none, as on a detached HEAD.
	GetBranch() string
	GetHead() string
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

var _ Client = (*git.Client)(nil)

const gitBinary = "git"

// New returns the backend for dir. Git is currently the only one.
func New(dir string) Client {
	return newGit(dir)
}

func newGit(dir string) *git.Client {
	return &git.Client{Binary: gitBinary, Dir: dir}
}
