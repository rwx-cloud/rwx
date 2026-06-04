package mocks

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/rwx-cloud/rwx/internal/git"
)

type Git struct {
	MockGetBranch              string
	MockGetHead                string
	MockGetHeadError           error
	MockGetCommit              string
	MockGetCommitError         error
	MockGetOriginUrl           string
	MockGeneratePatchFile      git.PatchFile
	MockGeneratePatchFileError error
	MockGeneratePatch          func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error)
	MockGenerateDirtyPatches   func() (git.DirtyPatches, error)
	MockHasCommit              func(sha string) bool
	MockCreateBundleFile       func(head string, excludes []string) (git.BundleFile, error)
	MockCreateShallowStatePack func(head string, excludes []string) (git.PackFile, error)
	MockApplyPatch             func(patch []byte) *exec.Cmd
	MockApplyPatchReject       func(patch []byte) *exec.Cmd
	MockIsInstalled            bool
	MockIsInsideWorkTree       bool
}

func (c *Git) GetBranch() string {
	return c.MockGetBranch
}

func (c *Git) GetHead() string {
	head, err := c.GetHeadCommit()
	if err != nil {
		return ""
	}
	return head
}

func (c *Git) GetHeadCommit() (string, error) {
	return c.MockGetHead, c.MockGetHeadError
}

func (c *Git) GetCommit() (string, error) {
	return c.MockGetCommit, c.MockGetCommitError
}

func (c *Git) GetOriginUrl() string {
	return c.MockGetOriginUrl
}

func (c *Git) GeneratePatchFile(destDir string, pathspec []string) (git.PatchFile, error) {
	if c.MockGeneratePatchFileError != nil {
		return git.PatchFile{}, c.MockGeneratePatchFileError
	}

	if c.MockGeneratePatchFile.Written {
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return git.PatchFile{}, err
		}

		sha, _ := c.GetCommit()
		path := filepath.Join(destDir, sha)
		if err := os.WriteFile(path, []byte("patch"), 0644); err != nil {
			return git.PatchFile{}, err
		}

		return git.PatchFile{
			Written:         c.MockGeneratePatchFile.Written,
			Path:            path,
			UntrackedFiles:  c.MockGeneratePatchFile.UntrackedFiles,
			LFSChangedFiles: c.MockGeneratePatchFile.LFSChangedFiles,
		}, nil
	}

	return c.MockGeneratePatchFile, nil
}

func (c *Git) GeneratePatch(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
	if c.MockGeneratePatch != nil {
		return c.MockGeneratePatch(pathspec)
	}
	return nil, nil, nil
}

func (c *Git) GenerateDirtyPatches() (git.DirtyPatches, error) {
	if c.MockGenerateDirtyPatches != nil {
		return c.MockGenerateDirtyPatches()
	}
	return git.DirtyPatches{}, nil
}

func (c *Git) HasCommit(sha string) bool {
	if c.MockHasCommit != nil {
		return c.MockHasCommit(sha)
	}
	return true
}

func (c *Git) CreateBundleFile(head string, excludes []string) (git.BundleFile, error) {
	if c.MockCreateBundleFile != nil {
		return c.MockCreateBundleFile(head, excludes)
	}
	return git.BundleFile{}, nil
}

func (c *Git) CreateShallowStatePack(head string, excludes []string) (git.PackFile, error) {
	if c.MockCreateShallowStatePack != nil {
		return c.MockCreateShallowStatePack(head, excludes)
	}
	return git.PackFile{}, nil
}

func (c *Git) ApplyPatch(patch []byte) *exec.Cmd {
	if c.MockApplyPatch != nil {
		return c.MockApplyPatch(patch)
	}
	return exec.Command("true")
}

func (c *Git) ApplyPatchReject(patch []byte) *exec.Cmd {
	if c.MockApplyPatchReject != nil {
		return c.MockApplyPatchReject(patch)
	}
	return exec.Command("true")
}

func (c *Git) IsInstalled() bool {
	return c.MockIsInstalled
}

func (c *Git) IsInsideWorkTree() bool {
	return c.MockIsInsideWorkTree
}
