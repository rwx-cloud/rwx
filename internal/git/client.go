package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Client struct {
	Binary string
	Dir    string
}

const defaultRemote = "origin"

func getRemote() string {
	if remote := os.Getenv("RWX_GIT_REMOTE"); remote != "" {
		return remote
	}
	return defaultRemote
}

func (c *Client) IsInstalled() bool {
	_, err := exec.LookPath(c.Binary)
	return err == nil
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

// RelativePathToTopLevel expresses path relative to the repository's top-level
// directory, so it can be used in a :(top)-anchored pathspec independent of the
// current working directory. It returns "" when the top level can't be
// determined or path lies outside the work tree.
func (c *Client) RelativePathToTopLevel(path string) string {
	topLevel := c.GetTopLevel()
	if topLevel == "" {
		return ""
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(c.Dir, path)
	}

	// Resolve symlinks so both paths share a base; e.g. on macOS /var is a
	// symlink to /private/var and git reports the resolved top-level path.
	if resolved, err := filepath.EvalSymlinks(topLevel); err == nil {
		topLevel = resolved
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}

	rel, err := filepath.Rel(topLevel, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}

	// Git pathspecs always use forward slashes, even on Windows where
	// filepath.Rel returns backslash-separated paths.
	return filepath.ToSlash(rel)
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

func (c *Client) GetHead() string {
	head, err := c.GetHeadCommit()
	if err != nil {
		return ""
	}
	return head
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

	remote := getRemote()

	// Check if remote exists — for detached HEAD, fall back to raw HEAD
	if c.GetRemoteUrl(remote) == "" {
		if c.GetBranch() == "" {
			return c.GetHead(), nil
		}
		return "", fmt.Errorf("no git remote named '%s' is configured (set RWX_GIT_REMOTE to use a different remote)", remote)
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
		return c.GetHead(), nil
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
		return c.GetHead(), nil
	}
	return "", fmt.Errorf("current branch has no commits in common with the '%s' remote (set RWX_GIT_REMOTE to use a different remote)", remote)
}

func CommitMismatchNote(head, runCommit string) string {
	if strings.HasPrefix(runCommit, head) || strings.HasPrefix(head, runCommit) {
		return ""
	}
	shortHead := head
	if len(shortHead) > 7 {
		shortHead = shortHead[:7]
	}
	shortCommit := runCommit
	if len(shortCommit) > 7 {
		shortCommit = shortCommit[:7]
	}
	return fmt.Sprintf("Note: you're currently on commit %s but the most recent run on this branch was for commit %s", shortHead, shortCommit)
}

func (c *Client) GetOriginUrl() string {
	return c.GetRemoteUrl(getRemote())
}

// RepoNameFromOriginUrl extracts the repository name from a git remote URL.
// For example, "git@github.com:rwx-cloud/rwx.git" returns "rwx".
func RepoNameFromOriginUrl(originUrl string) string {
	// Handle SSH-style URLs (git@github.com:rwx-cloud/rwx.git)
	if idx := strings.LastIndex(originUrl, ":"); idx != -1 && !strings.Contains(originUrl, "://") {
		originUrl = originUrl[idx+1:]
	}

	// Handle HTTPS-style URLs (https://github.com/rwx-cloud/rwx.git)
	if idx := strings.LastIndex(originUrl, "/"); idx != -1 {
		originUrl = originUrl[idx+1:]
	}

	return strings.TrimSuffix(originUrl, ".git")
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

type UntrackedFilesMetadata struct {
	Files []string
	Count int
}

type LFSChangedFilesMetadata struct {
	Files []string
	Count int
}

type PatchFile struct {
	Written         bool
	Path            string
	UntrackedFiles  UntrackedFilesMetadata
	LFSChangedFiles LFSChangedFilesMetadata
}

type DirtyPatches struct {
	Staged          []byte
	Unstaged        []byte
	Files           []string
	NewFiles        []string
	LFSChangedFiles *LFSChangedFilesMetadata
}

func (p DirtyPatches) Size() int {
	return len(p.Staged) + len(p.Unstaged)
}

type PushRefOptions struct {
	Remote  string
	Refspec string
	Env     []string
}

// patchResult holds the result of generating patch data
type patchResult struct {
	patch     []byte
	sha       string
	untracked UntrackedFilesMetadata
	lfs       LFSChangedFilesMetadata
	ok        bool
}

// generatePatchData generates patch data for working tree changes relative to the base commit on origin.
func (c *Client) generatePatchData(pathspec []string) patchResult {
	sha, err := c.GetCommit()
	if sha == "" || err != nil {
		return patchResult{}
	}

	diffArgs := []string{"diff", sha, "--name-only"}
	if len(pathspec) > 0 {
		diffArgs = append(diffArgs, "--")
		diffArgs = append(diffArgs, pathspec...)
	}
	cmd := exec.Command(c.Binary, diffArgs...)
	cmd.Dir = c.Dir

	files, err := cmd.Output()
	if err != nil {
		return patchResult{}
	}

	lfsChanged, err := c.lfsFilesForPaths(strings.Split(strings.TrimSpace(string(files)), "\n"))
	if err != nil {
		return patchResult{}
	}

	if lfsChanged.Count > 0 {
		return patchResult{
			sha: sha,
			lfs: lfsChanged,
			ok:  true,
		}
	}

	lsFilesArgs := []string{"ls-files", "--others", "--exclude-standard"}
	if len(pathspec) > 0 {
		lsFilesArgs = append(lsFilesArgs, "--")
		lsFilesArgs = append(lsFilesArgs, pathspec...)
	}
	cmd = exec.Command(c.Binary, lsFilesArgs...)
	cmd.Dir = c.Dir

	untracked, err := cmd.Output()
	if err != nil {
		return patchResult{}
	}

	untrackedFiles := strings.Fields(string(untracked))

	patchArgs := []string{"diff", sha, "-p", "--binary"}
	if len(pathspec) > 0 {
		patchArgs = append(patchArgs, "--")
		patchArgs = append(patchArgs, pathspec...)
	}
	cmd = exec.Command(c.Binary, patchArgs...)
	cmd.Dir = c.Dir

	patch, err := cmd.Output()
	if err != nil {
		return patchResult{}
	}

	return patchResult{
		patch: patch,
		sha:   sha,
		untracked: UntrackedFilesMetadata{
			Files: untrackedFiles,
			Count: len(untrackedFiles),
		},
		ok: true,
	}
}

func (c *Client) GeneratePatchFile(destDir string, pathspec []string) (PatchFile, error) {
	cleanup, err := c.AddUntrackedFilesForPatch(pathspec)
	if err != nil {
		cleanup = func() {}
	}
	defer cleanup()

	data := c.generatePatchData(pathspec)
	if !data.ok {
		return PatchFile{}, fmt.Errorf("unable to generate patch data")
	}

	if data.lfs.Count > 0 {
		return PatchFile{LFSChangedFiles: data.lfs}, nil
	}

	if len(data.patch) == 0 {
		return PatchFile{UntrackedFiles: data.untracked}, nil
	}

	outputPath := filepath.Join(destDir, data.sha)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return PatchFile{}, fmt.Errorf("unable to create patch directory: %w", err)
	}

	if err := os.WriteFile(outputPath, data.patch, 0644); err != nil {
		return PatchFile{}, fmt.Errorf("unable to write patch file: %w", err)
	}

	return PatchFile{
		Written:        true,
		Path:           outputPath,
		UntrackedFiles: data.untracked,
	}, nil
}

// AddUntrackedFilesForPatch temporarily adds untracked files with intent-to-add
// so they appear in git diff. The pathspec must match the one used to generate
// the patch, so that intent-to-add coverage is scoped identically to the diff
// (e.g. a :(top) pathspec captures untracked files across the whole repo, not
// just those under the current working directory). Returns a cleanup function
// to undo the add.
func (c *Client) AddUntrackedFilesForPatch(pathspec []string) (cleanup func(), err error) {
	// Get untracked files
	lsArgs := []string{"ls-files", "--others", "--exclude-standard"}
	if len(pathspec) > 0 {
		lsArgs = append(lsArgs, "--")
		lsArgs = append(lsArgs, pathspec...)
	}
	cmd := exec.Command(c.Binary, lsArgs...)
	cmd.Dir = c.Dir
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Split on newlines (not Fields) to handle filenames with spaces
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var files []string
	for _, line := range lines {
		if line != "" {
			files = append(files, line)
		}
	}

	if len(files) == 0 {
		return func() {}, nil // No untracked files, no-op cleanup
	}

	// Add with intent-to-add
	args := append([]string{"add", "-N", "--"}, files...)
	cmd = exec.Command(c.Binary, args...)
	cmd.Dir = c.Dir
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	// Return cleanup function
	cleanup = func() {
		args := append([]string{"reset", "HEAD", "--"}, files...)
		cmd := exec.Command(c.Binary, args...)
		cmd.Dir = c.Dir
		_ = cmd.Run() // Best effort cleanup
	}

	return cleanup, nil
}

// GeneratePatch returns patch bytes for working tree changes relative to the base commit on origin.
// Returns (nil, nil, nil) if no changes or unable to generate patch.
func (c *Client) GeneratePatch(pathspec []string) ([]byte, *LFSChangedFilesMetadata, error) {
	// Add untracked files temporarily so they appear in the diff
	cleanup, err := c.AddUntrackedFilesForPatch(pathspec)
	if err != nil {
		// Non-fatal: proceed without untracked files
		cleanup = func() {}
	}
	defer cleanup()

	data := c.generatePatchData(pathspec)
	if !data.ok {
		return nil, nil, nil
	}

	if data.lfs.Count > 0 {
		return nil, &data.lfs, nil
	}

	if len(data.patch) == 0 {
		return nil, nil, nil
	}

	return data.patch, nil, nil
}

func (c *Client) GenerateDirtyPatches() (DirtyPatches, error) {
	// Anchor the untracked-file lookup to the repository root with git's :(top)
	// magic so the sandbox sync captures untracked files everywhere in the repo,
	// not just under the current working directory. Falls back to the cwd-scoped
	// lookup if the top level can't be resolved.
	var untrackedPathspec []string
	if c.GetTopLevel() != "" {
		untrackedPathspec = []string{":(top)"}
	}
	cleanup, err := c.AddUntrackedFilesForPatch(untrackedPathspec)
	if err != nil {
		cleanup = func() {}
	}
	defer cleanup()

	files, err := c.changedFilesForDirtyPatch()
	if err != nil {
		return DirtyPatches{}, err
	}
	newFiles, err := c.newFilesForDirtyPatch()
	if err != nil {
		return DirtyPatches{}, err
	}

	lfsChangedFiles := []string{}
	for _, file := range files {
		cmd := exec.Command(c.Binary, "check-attr", "filter", "--", file)
		cmd.Dir = c.Dir

		attrs, err := cmd.CombinedOutput()
		if err != nil {
			return DirtyPatches{}, err
		}

		if strings.Contains(string(attrs), "filter: lfs") {
			parts := strings.SplitN(string(attrs), ":", 2)
			lfsFile := strings.TrimSpace(parts[0])
			lfsChangedFiles = append(lfsChangedFiles, lfsFile)
		}
	}

	if len(lfsChangedFiles) > 0 {
		return DirtyPatches{
			Files:    files,
			NewFiles: newFiles,
			LFSChangedFiles: &LFSChangedFilesMetadata{
				Files: lfsChangedFiles,
				Count: len(lfsChangedFiles),
			},
		}, nil
	}

	staged, err := c.diffBytes("diff", "--cached", "-p", "--binary")
	if err != nil {
		return DirtyPatches{}, err
	}
	unstaged, err := c.diffBytes("diff", "-p", "--binary")
	if err != nil {
		return DirtyPatches{}, err
	}

	return DirtyPatches{Staged: staged, Unstaged: unstaged, Files: files, NewFiles: newFiles}, nil
}

func (c *Client) changedFilesForDirtyPatch() ([]string, error) {
	seen := map[string]bool{}
	var files []string

	for _, args := range [][]string{
		{"diff", "--cached", "--name-only"},
		{"diff", "--name-only"},
	} {
		cmd := exec.Command(c.Binary, args...)
		cmd.Dir = c.Dir
		out, err := cmd.Output()
		if err != nil {
			return nil, err
		}

		for _, file := range strings.Split(strings.TrimSpace(string(out)), "\n") {
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
		{"diff", "--cached", "--name-only", "--diff-filter=A"},
		{"diff", "--name-only", "--diff-filter=A"},
	} {
		cmd := exec.Command(c.Binary, args...)
		cmd.Dir = c.Dir
		out, err := cmd.Output()
		if err != nil {
			return nil, err
		}

		for _, file := range strings.Split(strings.TrimSpace(string(out)), "\n") {
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

func (c *Client) lfsFilesForPaths(files []string) (LFSChangedFilesMetadata, error) {
	lfsChangedFiles := []string{}

	for _, file := range files {
		if file == "" {
			continue
		}

		cmd := exec.Command(c.Binary, "check-attr", "filter", "--", file)
		cmd.Dir = c.Dir

		attrs, err := cmd.CombinedOutput()
		if err != nil {
			return LFSChangedFilesMetadata{}, err
		}

		if strings.Contains(string(attrs), "filter: lfs") {
			parts := strings.SplitN(string(attrs), ":", 2)
			lfsFile := strings.TrimSpace(parts[0])
			lfsChangedFiles = append(lfsChangedFiles, lfsFile)
		}
	}

	return LFSChangedFilesMetadata{
		Files: lfsChangedFiles,
		Count: len(lfsChangedFiles),
	}, nil
}

func (c *Client) PushRef(opts PushRefOptions) error {
	if opts.Remote == "" {
		return fmt.Errorf("no remote provided")
	}
	if opts.Refspec == "" {
		return fmt.Errorf("no refspec provided")
	}

	cmd := exec.Command(c.Binary, "push", opts.Remote, opts.Refspec)
	cmd.Dir = c.Dir
	if len(opts.Env) > 0 {
		cmd.Env = append(os.Environ(), opts.Env...)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		output := strings.TrimSpace(string(out))
		if output != "" {
			return fmt.Errorf("git push failed: %s", output)
		}
		return fmt.Errorf("git push failed: %w", err)
	}

	return nil
}

// IsAncestor returns true if candidateSHA is an ancestor of (or equal to) headRef.
// Returns false on any error, including when not in a git repo.
func (c *Client) IsAncestor(candidateSHA, headRef string) bool {
	cmd := exec.Command(c.Binary, "merge-base", "--is-ancestor", candidateSHA, headRef)
	cmd.Dir = c.Dir
	return cmd.Run() == nil
}

// ApplyPatch returns an exec.Cmd that applies a patch to the working directory.
// The patch bytes should be provided to the command's stdin before running.
func (c *Client) ApplyPatch(patch []byte) *exec.Cmd {
	cmd := exec.Command(c.Binary, "apply", "--allow-empty", "-")
	cmd.Dir = c.Dir
	cmd.Stdin = bytes.NewReader(patch)
	return cmd
}

// ApplyPatchReject returns an exec.Cmd that applies a patch with --reject,
// which applies hunks that succeed and writes .rej files for hunks that fail.
func (c *Client) ApplyPatchReject(patch []byte) *exec.Cmd {
	cmd := exec.Command(c.Binary, "apply", "--reject", "--allow-empty", "-")
	cmd.Dir = c.Dir
	cmd.Stdin = bytes.NewReader(patch)
	return cmd
}
