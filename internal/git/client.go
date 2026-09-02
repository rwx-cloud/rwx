package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/rwx-cloud/rwx/internal/vcs/vcstypes"
)

type Client struct {
	Binary string
	Dir    string
}

// MissingDependency returns the name of the executable this backend needs but
// cannot find, or "" when everything is present.
func (c *Client) MissingDependency() string {
	if _, err := exec.LookPath(c.Binary); err != nil {
		return "git"
	}
	return ""
}

func (c *Client) IsInsideWorkTree() bool {
	cmd := exec.Command(c.Binary, "rev-parse", "--is-inside-work-tree")
	cmd.Dir = c.Dir
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

func (c *Client) GetTopLevel() string {
	cmd := exec.Command(c.Binary, "rev-parse", "--show-toplevel")
	cmd.Dir = c.Dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (c *Client) GetBranch() string {
	cmd := exec.Command(c.Binary, "branch", "--show-current")
	cmd.Dir = c.Dir

	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	branch := strings.TrimSpace(string(out))
	return branch
}

func (c *Client) GetHeadCommit() (string, error) {
	cmd := exec.Command(c.Binary, "rev-parse", "--verify", "HEAD^{commit}")
	cmd.Dir = c.Dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("unable to resolve HEAD: %s", msg)
	}

	head := strings.TrimSpace(string(out))
	if head == "" {
		return "", fmt.Errorf("unable to resolve HEAD")
	}

	return head, nil
}

// GetLastCommit is HEAD; git's working copy is not a commit.
func (c *Client) GetLastCommit() (string, error) {
	return c.GetHeadCommit()
}

func (c *Client) GetShortHead() string {
	cmd := exec.Command(c.Binary, "rev-parse", "--short", "HEAD")
	cmd.Dir = c.Dir

	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(out))
}

func (c *Client) GetCommit() (string, error) {
	// Check if HEAD resolves first
	checkHead := exec.Command(c.Binary, "rev-parse", "HEAD")
	checkHead.Dir = c.Dir
	if err := checkHead.Run(); err != nil {
		if c.GetBranch() == "" {
			// Not a git repository or no commits — silent no-op for detached HEAD
			return "", nil
		}
		return "", fmt.Errorf("current branch has no commits")
	}

	remote := vcstypes.ConfiguredRemote()

	// Check if remote exists — for detached HEAD, fall back to raw HEAD
	if c.GetRemoteUrl(remote) == "" {
		if c.GetBranch() == "" {
			return c.GetHeadCommit()
		}
		return "", vcstypes.MissingRemoteError(remote)
	}

	// Find commits on HEAD that aren't on any remote ref, with boundary markers.
	// This works for both named branches and detached HEAD.
	cmd := exec.Command(c.Binary, "rev-list", "HEAD", "--not", "--remotes="+remote, "--boundary")
	cmd.Dir = c.Dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-list failed: %s", strings.TrimSpace(string(out)))
	}

	output := strings.TrimSpace(string(out))

	// Empty output means HEAD is on an origin ref (no divergence) - return HEAD
	if output == "" {
		return c.GetHeadCommit()
	}

	// First line starting with "-" is the boundary (closest merge-base)
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "-") {
			return line[1:], nil
		}
	}

	// Output but no boundary means no common ancestor
	if c.GetBranch() == "" {
		// Detached HEAD with no remote ancestor — fall back to raw HEAD so
		// the caller can still attempt the operation (sync will patch on top).
		return c.GetHeadCommit()
	}
	return "", vcstypes.NoCommonAncestorError("current branch", remote)
}

func (c *Client) GetOriginUrl() string {
	return c.GetRemoteUrl(vcstypes.ConfiguredRemote())
}

func (c *Client) GetRemoteUrl(remote string) string {
	cmd := exec.Command(c.Binary, "remote", "get-url", remote)
	cmd.Dir = c.Dir

	url, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(url))
}

// generatePatchData generates patch data for working tree changes relative to the base commit on origin.
// On a git command failure it returns a *vcstypes.PatchError identifying which command failed and why.
func (c *Client) generatePatchData(pathspec []string) (vcstypes.PatchResult, *vcstypes.PatchError) {
	sha, err := c.GetCommit()
	if sha == "" || err != nil {
		// GetCommit failures are pre-filtered upstream in InitiateRun; treat as
		// "nothing to patch" here rather than a git command failure.
		return vcstypes.PatchResult{}, nil
	}

	diffArgs := []string{"diff", "-z", "--name-only", sha}
	if len(pathspec) > 0 {
		diffArgs = append(diffArgs, "--")
		diffArgs = append(diffArgs, pathspec...)
	}
	cmd := exec.Command(c.Binary, diffArgs...)
	cmd.Dir = c.Dir

	files, err := cmd.Output()
	if err != nil {
		return vcstypes.PatchResult{}, vcstypes.NewPatchError("diff_name_only", "git diff --name-only", err, "")
	}

	lfsChanged, lfsErr := c.lfsFilesForPaths(vcstypes.SplitNULPaths(files))
	if lfsErr != nil {
		return vcstypes.PatchResult{}, lfsErr
	}

	if lfsChanged.Count > 0 {
		return vcstypes.PatchResult{
			SHA: sha,
			LFS: lfsChanged,
			OK:  true,
		}, nil
	}

	lsFilesArgs := []string{"ls-files", "-z", "--others", "--exclude-standard"}
	if len(pathspec) > 0 {
		lsFilesArgs = append(lsFilesArgs, "--")
		lsFilesArgs = append(lsFilesArgs, pathspec...)
	}
	cmd = exec.Command(c.Binary, lsFilesArgs...)
	cmd.Dir = c.Dir

	untracked, err := cmd.Output()
	if err != nil {
		return vcstypes.PatchResult{}, vcstypes.NewPatchError("ls_files", "git ls-files --others --exclude-standard", err, "")
	}

	untrackedFiles := vcstypes.SplitNULPaths(untracked)

	patchArgs := []string{"diff", sha, "-p", "--binary"}
	if len(pathspec) > 0 {
		patchArgs = append(patchArgs, "--")
		patchArgs = append(patchArgs, pathspec...)
	}
	cmd = exec.Command(c.Binary, patchArgs...)
	cmd.Dir = c.Dir

	patch, err := cmd.Output()
	if err != nil {
		return vcstypes.PatchResult{}, vcstypes.NewPatchError("diff_patch", "git diff -p --binary", err, "")
	}

	return vcstypes.PatchResult{
		Patch: patch,
		SHA:   sha,
		Untracked: vcstypes.UntrackedFilesMetadata{
			Files: untrackedFiles,
			Count: len(untrackedFiles),
		},
		OK: true,
	}, nil
}

func (c *Client) GeneratePatchFile(destDir string, pathspec []string) (vcstypes.PatchFile, error) {
	cleanup, err := c.AddUntrackedFilesForPatch()
	if err != nil {
		cleanup = func() {}
	}
	defer cleanup()

	data, patchErr := c.generatePatchData(pathspec)
	if patchErr != nil {
		return vcstypes.PatchFile{}, patchErr
	}

	return data.WriteFile(destDir)
}

// AddUntrackedFilesForPatch temporarily adds untracked files with intent-to-add
// so they appear in git diff. Returns a cleanup function to undo the add.
func (c *Client) AddUntrackedFilesForPatch() (cleanup func(), err error) {
	dir := c.applyDir()

	// Get untracked files
	cmd := exec.Command(c.Binary, "ls-files", "-z", "--others", "--exclude-standard")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	files := vcstypes.SplitNULPaths(output)

	if len(files) == 0 {
		return func() {}, nil // No untracked files, no-op cleanup
	}

	// Add with intent-to-add
	args := append([]string{"add", "-N", "--"}, files...)
	cmd = exec.Command(c.Binary, args...)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	// Return cleanup function
	cleanup = func() {
		args := append([]string{"reset", "HEAD", "--"}, files...)
		cmd := exec.Command(c.Binary, args...)
		cmd.Dir = dir
		_ = cmd.Run() // Best effort cleanup
	}

	return cleanup, nil
}

// GeneratePatch returns patch bytes for working tree changes relative to the base commit on origin.
// Returns (nil, nil, nil) if no changes or unable to generate patch.
func (c *Client) GeneratePatch(pathspec []string) ([]byte, *vcstypes.LFSChangedFilesMetadata, error) {
	// Add untracked files temporarily so they appear in the diff
	cleanup, err := c.AddUntrackedFilesForPatch()
	if err != nil {
		// Non-fatal: proceed without untracked files
		cleanup = func() {}
	}
	defer cleanup()

	data, patchErr := c.generatePatchData(pathspec)
	if patchErr != nil {
		return nil, nil, nil
	}

	patch, lfs := data.Bytes()
	return patch, lfs, nil
}

func (c *Client) GenerateDirtyPatches() (vcstypes.DirtyPatches, error) {
	cleanup, err := c.AddUntrackedFilesForPatch()
	if err != nil {
		cleanup = func() {}
	}
	defer cleanup()

	files, err := c.changedFilesForDirtyPatch()
	if err != nil {
		return vcstypes.DirtyPatches{}, err
	}
	newFiles, err := c.newFilesForDirtyPatch()
	if err != nil {
		return vcstypes.DirtyPatches{}, err
	}

	lfsChangedFiles := []string{}
	dir := c.applyDir()
	for _, file := range files {
		cmd := exec.Command(c.Binary, "check-attr", "filter", "--", file)
		cmd.Dir = dir

		attrs, err := cmd.CombinedOutput()
		if err != nil {
			return vcstypes.DirtyPatches{}, err
		}

		if strings.Contains(string(attrs), "filter: lfs") {
			lfsChangedFiles = append(lfsChangedFiles, file)
		}
	}

	if len(lfsChangedFiles) > 0 {
		return vcstypes.DirtyPatches{
			Files:    files,
			NewFiles: newFiles,
			LFSChangedFiles: &vcstypes.LFSChangedFilesMetadata{
				Files: lfsChangedFiles,
				Count: len(lfsChangedFiles),
			},
		}, nil
	}

	staged, err := c.diffBytes("diff", "--cached", "-p", "--binary", "--no-renames")
	if err != nil {
		return vcstypes.DirtyPatches{}, err
	}
	unstaged, err := c.diffBytes("diff", "-p", "--binary", "--no-renames")
	if err != nil {
		return vcstypes.DirtyPatches{}, err
	}

	return vcstypes.DirtyPatches{Staged: staged, Unstaged: unstaged, Files: files, NewFiles: newFiles}, nil
}

func (c *Client) changedFilesForDirtyPatch() ([]string, error) {
	seen := map[string]bool{}
	var files []string

	for _, args := range [][]string{
		{"diff", "--cached", "-z", "--name-only", "--no-renames"},
		{"diff", "-z", "--name-only", "--no-renames"},
	} {
		cmd := exec.Command(c.Binary, args...)
		cmd.Dir = c.Dir
		out, err := cmd.Output()
		if err != nil {
			return nil, err
		}

		for _, file := range vcstypes.SplitNULPaths(out) {
			if file == "" || seen[file] {
				continue
			}
			seen[file] = true
			files = append(files, file)
		}
	}

	return files, nil
}

func (c *Client) newFilesForDirtyPatch() ([]string, error) {
	seen := map[string]bool{}
	var files []string

	for _, args := range [][]string{
		{"diff", "--cached", "-z", "--name-only", "--diff-filter=A", "--no-renames"},
		{"diff", "-z", "--name-only", "--diff-filter=A", "--no-renames"},
	} {
		cmd := exec.Command(c.Binary, args...)
		cmd.Dir = c.Dir
		out, err := cmd.Output()
		if err != nil {
			return nil, err
		}

		for _, file := range vcstypes.SplitNULPaths(out) {
			if file == "" || seen[file] {
				continue
			}
			seen[file] = true
			files = append(files, file)
		}
	}

	return files, nil
}

func (c *Client) diffBytes(args ...string) ([]byte, error) {
	cmd := exec.Command(c.Binary, args...)
	cmd.Dir = c.Dir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) HasCommit(sha string) bool {
	if sha == "" {
		return false
	}
	cmd := exec.Command(c.Binary, "cat-file", "-e", sha+"^{commit}")
	cmd.Dir = c.Dir
	return cmd.Run() == nil
}

func (c *Client) lfsFilesForPaths(files []string) (vcstypes.LFSChangedFilesMetadata, *vcstypes.PatchError) {
	dir := c.applyDir()

	return vcstypes.LFSFilesForPaths(files, func(args ...string) (*exec.Cmd, error) {
		cmd := exec.Command(c.Binary, args...)
		cmd.Dir = dir
		return cmd, nil
	})
}

func (c *Client) PushRef(opts vcstypes.PushRefOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}

	cmd := exec.Command(c.Binary, "push", opts.Remote, opts.Refspec)
	cmd.Dir = c.Dir
	return vcstypes.RunPush(cmd, opts.Env)
}

// IsAncestor returns true if candidateSHA is an ancestor of (or equal to) headRef.
// Returns false on any error, including when not in a git repo.
func (c *Client) IsAncestor(candidateSHA, headRef string) bool {
	cmd := exec.Command(c.Binary, "merge-base", "--is-ancestor", candidateSHA, headRef)
	cmd.Dir = c.Dir
	return cmd.Run() == nil
}

func (c *Client) applyDir() string {
	if topLevel := c.GetTopLevel(); topLevel != "" {
		return topLevel
	}
	return c.Dir
}

// ApplyPatch returns an exec.Cmd that applies a patch to the working directory.
// The patch bytes should be provided to the command's stdin before running.
func (c *Client) ApplyPatch(patch []byte) *exec.Cmd {
	cmd := exec.Command(c.Binary, "apply", "--allow-empty", "-")
	cmd.Dir = c.applyDir()
	cmd.Stdin = bytes.NewReader(patch)
	return cmd
}

// ApplyPatchReject returns an exec.Cmd that applies a patch with --reject,
// which applies hunks that succeed and writes .rej files for hunks that fail.
func (c *Client) ApplyPatchReject(patch []byte) *exec.Cmd {
	cmd := exec.Command(c.Binary, "apply", "--reject", "--allow-empty", "-")
	cmd.Dir = c.applyDir()
	cmd.Stdin = bytes.NewReader(patch)
	return cmd
}
