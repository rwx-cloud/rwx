package mocks

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/rwx-cloud/rwx/internal/vcs"
)

type VCS struct {
	MockGetBranch              string
	MockGetHead                string
	MockGetHeadError           error
	MockGetLastCommit          string
	MockGetLastCommitError     error
	MockGetTopLevel            string
	MockGetCommit              string
	MockGetCommitError         error
	MockGetOriginUrl           string
	MockGeneratePatchFile      vcs.PatchFile
	MockGeneratePatchFileError error
	MockGeneratePatchFileFunc  func(destDir string, pathspec []string) (vcs.PatchFile, error)
	MockGeneratePatchPathspec  []string
	MockGeneratePatch          func(pathspec []string) ([]byte, *vcs.LFSChangedFilesMetadata, error)
	MockGenerateDirtyPatches   func() (vcs.DirtyPatches, error)
	MockPushRef                func(opts vcs.PushRefOptions) error
	MockApplyPatch             func(patch []byte) *exec.Cmd
	MockApplyPatchReject       func(patch []byte) *exec.Cmd
	MockIsInsideWorkTree       bool
	MockMissingDependency      string
	MockGetShortHead           string
	MockIsAncestor             func(candidateSHA, headRef string) bool
}

func (c *VCS) GetBranch() string {
	return c.MockGetBranch
}

func (c *VCS) GetHeadCommit() (string, error) {
	return c.MockGetHead, c.MockGetHeadError
}

func (c *VCS) GetLastCommit() (string, error) {
	return c.MockGetLastCommit, c.MockGetLastCommitError
}

func (c *VCS) GetTopLevel() string {
	return c.MockGetTopLevel
}

func (c *VCS) GetCommit() (string, error) {
	return c.MockGetCommit, c.MockGetCommitError
}

func (c *VCS) GetOriginUrl() string {
	return c.MockGetOriginUrl
}

func (c *VCS) GeneratePatchFile(destDir string, pathspec []string) (vcs.PatchFile, error) {
	c.MockGeneratePatchPathspec = append([]string(nil), pathspec...)
	if c.MockGeneratePatchFileFunc != nil {
		return c.MockGeneratePatchFileFunc(destDir, pathspec)
	}

	if c.MockGeneratePatchFileError != nil {
		return vcs.PatchFile{}, c.MockGeneratePatchFileError
	}

	if c.MockGeneratePatchFile.Written {
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return vcs.PatchFile{}, err
		}

		sha, _ := c.GetCommit()
		path := filepath.Join(destDir, sha)
		if err := os.WriteFile(path, []byte("patch"), 0644); err != nil {
			return vcs.PatchFile{}, err
		}

		return vcs.PatchFile{
			Written:         c.MockGeneratePatchFile.Written,
			Path:            path,
			UntrackedFiles:  c.MockGeneratePatchFile.UntrackedFiles,
			LFSChangedFiles: c.MockGeneratePatchFile.LFSChangedFiles,
		}, nil
	}

	return c.MockGeneratePatchFile, nil
}

func (c *VCS) GeneratePatch(pathspec []string) ([]byte, *vcs.LFSChangedFilesMetadata, error) {
	if c.MockGeneratePatch != nil {
		return c.MockGeneratePatch(pathspec)
	}
	return nil, nil, nil
}

func (c *VCS) GenerateDirtyPatches() (vcs.DirtyPatches, error) {
	if c.MockGenerateDirtyPatches != nil {
		return c.MockGenerateDirtyPatches()
	}
	return vcs.DirtyPatches{}, nil
}

func (c *VCS) PushRef(opts vcs.PushRefOptions) error {
	if c.MockPushRef != nil {
		return c.MockPushRef(opts)
	}
	return nil
}

func (c *VCS) ApplyPatch(patch []byte) *exec.Cmd {
	if c.MockApplyPatch != nil {
		return c.MockApplyPatch(patch)
	}
	return exec.Command("true")
}

func (c *VCS) ApplyPatchReject(patch []byte) *exec.Cmd {
	if c.MockApplyPatchReject != nil {
		return c.MockApplyPatchReject(patch)
	}
	return exec.Command("true")
}

func (c *VCS) IsInsideWorkTree() bool {
	return c.MockIsInsideWorkTree
}

func (c *VCS) MissingDependency() string {
	return c.MockMissingDependency
}

func (c *VCS) GetShortHead() string {
	return c.MockGetShortHead
}

func (c *VCS) IsAncestor(candidateSHA, headRef string) bool {
	if c.MockIsAncestor != nil {
		return c.MockIsAncestor(candidateSHA, headRef)
	}
	return false
}

var _ vcs.Client = (*VCS)(nil)
