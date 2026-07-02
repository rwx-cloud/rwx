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

// PatchError identifies which git command failed while generating a patch,
// along with its exit code and stderr, so callers can show the underlying git
// error to the user and record a stable, PII-free bucket in telemetry.
type PatchError struct {
	Command  string // stable identifier for telemetry: diff_name_only, check_attr, ls_files, diff_patch
	Display  string // human-readable command, e.g. "git diff --name-only"
	Stderr   string // trimmed git stderr (the user's own repo data)
	ExitCode int    // process exit code, or -1 if the command never started
}

func (e *PatchError) Error() string {
	if e.Stderr == "" {
		return fmt.Sprintf("failed to generate patch (%s)", e.Display)
	}
	return fmt.Sprintf("failed to generate patch (%s): %s", e.Display, e.Stderr)
}

// Reason buckets the git stderr into a stable category for telemetry. Raw
// stderr must never be sent to telemetry — it embeds customer file paths,
// branch names, and repo layout.
func (e *PatchError) Reason() string {
	s := strings.ToLower(e.Stderr)
	switch {
	case strings.Contains(s, "bad object"), strings.Contains(s, "shallow"):
		return "shallow_clone"
	case strings.Contains(s, "beyond a symbolic link"):
		return "beyond_symlink"
	case strings.Contains(s, "external filter"), strings.Contains(s, "filter-process"):
		return "missing_external_filter"
	case strings.Contains(s, "signal: killed"), strings.Contains(s, "out of memory"), strings.Contains(s, "cannot allocate memory"):
		return "oom_killed"
	default:
		return "unknown"
	}
}

// newPatchError builds a PatchError from a failed exec, extracting the exit
// code and stderr from an *exec.ExitError when available. fallbackStderr is
// used when the error doesn't carry captured stderr (e.g. CombinedOutput).
func newPatchError(command, display string, err error, fallbackStderr string) *PatchError {
	pe := &PatchError{Command: command, Display: display, ExitCode: -1}

	if exitErr, ok := err.(*exec.ExitError); ok {
		pe.ExitCode = exitErr.ExitCode()
		pe.Stderr = strings.TrimSpace(string(exitErr.Stderr))
	}

	if pe.Stderr == "" {
		pe.Stderr = strings.TrimSpace(fallbackStderr)
	}
	if pe.Stderr == "" {
		pe.Stderr = strings.TrimSpace(err.Error())
	}

	return pe
}

// generatePatchData generates patch data for working tree changes relative to the base commit on origin.
// On a git command failure it returns a *PatchError identifying which command failed and why.
func (c *Client) generatePatchData(pathspec []string) (patchResult, *PatchError) {
	sha, err := c.GetCommit()
	if sha == "" || err != nil {
		// GetCommit failures are pre-filtered upstream in InitiateRun; treat as
		// "nothing to patch" here rather than a git command failure.
		return patchResult{}, nil
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
		return patchResult{}, newPatchError("diff_name_only", "git diff --name-only", err, "")
	}

	lfsChanged, lfsErr := c.lfsFilesForPaths(splitNULPaths(files))
	if lfsErr != nil {
		return patchResult{}, lfsErr
	}

	if lfsChanged.Count > 0 {
		return patchResult{
			sha: sha,
			lfs: lfsChanged,
			ok:  true,
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
		return patchResult{}, newPatchError("ls_files", "git ls-files --others --exclude-standard", err, "")
	}

	untrackedFiles := splitNULPaths(untracked)

	patchArgs := []string{"diff", sha, "-p", "--binary"}
	if len(pathspec) > 0 {
		patchArgs = append(patchArgs, "--")
		patchArgs = append(patchArgs, pathspec...)
	}
	cmd = exec.Command(c.Binary, patchArgs...)
	cmd.Dir = c.Dir

	patch, err := cmd.Output()
	if err != nil {
		return patchResult{}, newPatchError("diff_patch", "git diff -p --binary", err, "")
	}

	return patchResult{
		patch: patch,
		sha:   sha,
		untracked: UntrackedFilesMetadata{
			Files: untrackedFiles,
			Count: len(untrackedFiles),
		},
		ok: true,
	}, nil
}

func (c *Client) GeneratePatchFile(destDir string, pathspec []string) (PatchFile, error) {
	cleanup, err := c.AddUntrackedFilesForPatch()
	if err != nil {
		cleanup = func() {}
	}
	defer cleanup()

	data, patchErr := c.generatePatchData(pathspec)
	if patchErr != nil {
		return PatchFile{}, patchErr
	}
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

	files := splitNULPaths(output)

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
func (c *Client) GeneratePatch(pathspec []string) ([]byte, *LFSChangedFilesMetadata, error) {
	// Add untracked files temporarily so they appear in the diff
	cleanup, err := c.AddUntrackedFilesForPatch()
	if err != nil {
		// Non-fatal: proceed without untracked files
		cleanup = func() {}
	}
	defer cleanup()

	data, patchErr := c.generatePatchData(pathspec)
	if patchErr != nil || !data.ok {
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
	cleanup, err := c.AddUntrackedFilesForPatch()
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
	dir := c.applyDir()
	for _, file := range files {
		cmd := exec.Command(c.Binary, "check-attr", "filter", "--", file)
		cmd.Dir = dir

		attrs, err := cmd.CombinedOutput()
		if err != nil {
			return DirtyPatches{}, err
		}

		if strings.Contains(string(attrs), "filter: lfs") {
			lfsChangedFiles = append(lfsChangedFiles, file)
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

	staged, err := c.diffBytes("diff", "--cached", "-p", "--binary", "--no-renames")
	if err != nil {
		return DirtyPatches{}, err
	}
	unstaged, err := c.diffBytes("diff", "-p", "--binary", "--no-renames")
	if err != nil {
		return DirtyPatches{}, err
	}

	return DirtyPatches{Staged: staged, Unstaged: unstaged, Files: files, NewFiles: newFiles}, nil
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

		for _, file := range splitNULPaths(out) {
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

		for _, file := range splitNULPaths(out) {
			if file == "" || seen[file] {
				continue
			}
			seen[file] = true
			files = append(files, file)
		}
	}

	return files, nil
}

func splitNULPaths(output []byte) []string {
	if len(output) == 0 {
		return []string{}
	}

	parts := bytes.Split(output, []byte{0})
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		paths = append(paths, string(part))
	}
	return paths
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

func (c *Client) lfsFilesForPaths(files []string) (LFSChangedFilesMetadata, *PatchError) {
	lfsChangedFiles := []string{}
	dir := c.applyDir()

	for _, file := range files {
		if file == "" {
			continue
		}

		cmd := exec.Command(c.Binary, "check-attr", "filter", "--", file)
		cmd.Dir = dir

		// CombinedOutput mixes stderr into attrs, so pass it as the fallback
		// stderr for the PatchError (the *exec.ExitError won't carry .Stderr).
		attrs, err := cmd.CombinedOutput()
		if err != nil {
			return LFSChangedFilesMetadata{}, newPatchError("check_attr", "git check-attr filter", err, string(attrs))
		}

		if strings.Contains(string(attrs), "filter: lfs") {
			lfsChangedFiles = append(lfsChangedFiles, file)
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

	// --no-thin sends a self-contained pack: the sandbox repo is a shallow/partial
	// clone that lacks the delta base objects a thin pack would reference, which
	// otherwise fails with "unresolved deltas left after unpacking".
	cmd := exec.Command(c.Binary, "push", "--no-thin", opts.Remote, opts.Refspec)
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
