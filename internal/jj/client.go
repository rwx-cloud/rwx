package jj

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rwx-cloud/rwx/internal/vcs/vcstypes"
)

type Client struct {
	Binary    string
	GitBinary string
	Dir       string
	// Stderr receives warnings jj reports while still exiting successfully.
	// Nil discards them.
	Stderr io.Writer
}

const (
	workingCopy      = "@"
	commitIDTemplate = `commit_id ++ "\n"`
)

func (c *Client) exec(globals, args []string) (string, string, error) {
	full := append([]string{"--no-pager", "--color=never", "--quiet"}, globals...)
	cmd := exec.Command(c.Binary, append(full, args...)...)
	cmd.Dir = c.Dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	trimmedStderr := strings.TrimSpace(stderr.String())
	c.reportSnapshotRefusals(trimmedStderr)

	return strings.TrimSpace(stdout.String()), trimmedStderr, err
}

// jj skips files over snapshot.max-new-file-size (1MiB) and still exits 0, so
// without forwarding this the file is just missing from every patch.
const snapshotRefusalPrefix = "Refused to snapshot some files"

func (c *Client) reportSnapshotRefusals(stderr string) {
	if c.Stderr == nil || !strings.Contains(stderr, snapshotRefusalPrefix) {
		return
	}

	for _, line := range strings.Split(stderr, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Hint:") {
			break
		}
		fmt.Fprintln(c.Stderr, line)
	}
	fmt.Fprintln(c.Stderr, "These files are excluded from RWX patches and sandbox syncs. Raise jj's snapshot.max-new-file-size or add them to .gitignore.")
}

func (c *Client) run(args ...string) (string, string, error) {
	return c.exec(nil, args)
}

func (c *Client) runStale(args ...string) (string, string, error) {
	return c.exec([]string{"--ignore-working-copy"}, args)
}

func (c *Client) resolve(revset, template string) (string, string, error) {
	return c.run("log", "-r", revset, "--no-graph", "-T", template)
}

func (c *Client) resolveStale(revset, template string) (string, string, error) {
	return c.runStale("log", "-r", revset, "--no-graph", "-T", template)
}

func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func toRevset(ref string) string {
	if ref == "HEAD" {
		return workingCopy
	}
	return ref
}

func quoteRevsetString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// MissingDependency reports both binaries, because the jj backend reaches the
// object store through git.
func (c *Client) MissingDependency() string {
	if _, err := exec.LookPath(c.Binary); err != nil {
		return "jj"
	}
	if _, err := exec.LookPath(c.GitBinary); err != nil {
		return "git"
	}
	return ""
}

func (c *Client) IsInsideWorkTree() bool {
	return c.GetTopLevel() != ""
}

func (c *Client) GetTopLevel() string {
	out, _, err := c.runStale("root")
	if err != nil {
		return ""
	}
	return firstLine(out)
}

func (c *Client) applyDir() string {
	if root := c.GetTopLevel(); root != "" {
		return root
	}
	return c.Dir
}

// GetBranch counts only a bookmark on @ itself. jj does not advance bookmarks as
// you work, so a bookmark among @'s ancestors is a stale label for a commit you
// have already moved past; reporting it would attribute runs and sandbox
// sessions to a branch the change isn't on. Returning "" instead routes callers
// through the same detached-HEAD handling git already uses.
func (c *Client) GetBranch() string {
	out, _, err := c.resolveStale(workingCopy, `local_bookmarks.map(|b| b.name()).join("\n") ++ "\n"`)
	if err != nil {
		return ""
	}
	return firstLine(out)
}

func (c *Client) GetHead() string {
	head, err := c.GetHeadCommit()
	if err != nil {
		return ""
	}
	return head
}

func (c *Client) GetHeadCommit() (string, error) {
	out, stderr, err := c.resolve(workingCopy, commitIDTemplate)
	if err != nil {
		msg := stderr
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("unable to resolve HEAD: %s", msg)
	}

	head := firstLine(out)
	if head == "" {
		return "", fmt.Errorf("unable to resolve HEAD")
	}

	return head, nil
}

// GetShortHead reports a change id, not a commit id. It backs the
// "detached@<id>" sandbox session key, so it has to stay put while the working
// copy is edited: jj rewrites @ in place on every snapshot, and the replaced
// commit is not an ancestor of its replacement, so a commit-id key would miss on
// every edit and strand a sandbox per keystroke. A change id survives those
// rewrites and still resolves as a revset, which is what the ancestry fallback
// in the sandbox storage needs.
func (c *Client) GetShortHead() string {
	out, _, err := c.resolve(workingCopy, `change_id.short(7) ++ "\n"`)
	if err != nil {
		return ""
	}
	return firstLine(out)
}

// GetCommit returns the base commit on the remote that a run is recorded
// against. Resolving it needs no snapshot — rewriting @ in place leaves its
// ancestors alone — so the whole happy path reads stale and reporting commands
// can call it without mutating the repository. Only the fallbacks below, which
// answer with @ itself, take the snapshot.
func (c *Client) GetCommit() (string, error) {
	if out, _, err := c.resolveStale(workingCopy, commitIDTemplate); err != nil || firstLine(out) == "" {
		return "", nil
	}

	remote := vcstypes.ConfiguredRemote()
	remotes := c.remoteUrls()
	if remotes[remote] == "" {
		// With no bookmark on @ there is no named position to complain about, so
		// answer with @ and let the caller patch on top. This mirrors the git
		// backend, which falls back the same way on a detached HEAD.
		if len(remotes) == 0 || c.GetBranch() == "" {
			return c.GetHeadCommit()
		}
		return "", fmt.Errorf("no git remote named '%s' is configured (set RWX_GIT_REMOTE to use a different remote)", remote)
	}

	// root() is an ancestor of every commit, so it must be excluded or unrelated
	// histories would resolve to jj's all-zeros virtual root instead of failing.
	revset := fmt.Sprintf("heads((::@ & ::remote_bookmarks(remote=exact:%s)) ~ root())", quoteRevsetString(remote))
	out, stderr, err := c.resolveStale(revset, commitIDTemplate)
	if err != nil {
		msg := stderr
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("jj log failed: %s", msg)
	}

	base := firstLine(out)
	if base == "" {
		if c.GetBranch() == "" {
			return c.GetHeadCommit()
		}
		return "", fmt.Errorf("bookmark '%s' has no commits in common with the '%s' remote (set RWX_GIT_REMOTE to use a different remote)", c.GetBranch(), remote)
	}

	return base, nil
}

func (c *Client) GetOriginUrl() string {
	return c.GetRemoteUrl(vcstypes.ConfiguredRemote())
}

func (c *Client) GetRemoteUrl(remote string) string {
	return c.remoteUrls()[remote]
}

func (c *Client) remoteUrls() map[string]string {
	out, _, err := c.runStale("git", "remote", "list")
	if err != nil {
		return nil
	}

	remotes := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		name, url, ok := strings.Cut(strings.TrimSpace(line), " ")
		if ok {
			remotes[name] = strings.TrimSpace(url)
		}
	}

	return remotes
}

// store locates the git repository backing a jj repo, along with the work tree
// its commands should run in. Resolving it costs a `jj root` plus a couple of
// file reads, so callers that inspect many files resolve it once and reuse it.
type store struct {
	gitDir   string
	workTree string
}

func (c *Client) resolveStore() (store, error) {
	root := c.GetTopLevel()
	if root == "" {
		return store{}, fmt.Errorf("not inside a jj repository")
	}

	gitDir, err := gitDirForRoot(root)
	if err != nil {
		return store{}, err
	}

	return store{gitDir: gitDir, workTree: root}, nil
}

func gitDirForRoot(root string) (string, error) {
	repoDir := filepath.Join(root, ".jj", "repo")
	if info, err := os.Stat(repoDir); err == nil && !info.IsDir() {
		data, err := os.ReadFile(repoDir)
		if err != nil {
			return "", fmt.Errorf("unable to read %s: %w", repoDir, err)
		}
		target := strings.TrimSpace(string(data))
		if target == "" {
			return "", fmt.Errorf("%s is empty", repoDir)
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(root, ".jj", target)
		}
		repoDir = target
	}

	storeDir := filepath.Join(repoDir, "store")
	data, err := os.ReadFile(filepath.Join(storeDir, "git_target"))
	if err != nil {
		return "", fmt.Errorf("unable to locate the git store for this jj repository: %w", err)
	}

	target := strings.TrimSpace(string(data))
	if target == "" {
		return "", fmt.Errorf("unable to locate the git store for this jj repository")
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(storeDir, target)
	}

	return filepath.Clean(target), nil
}

// storeCmd builds a single git command against the backing store. Use
// resolveStore plus storeCmdIn instead when issuing more than one.
func (c *Client) storeCmd(args ...string) (*exec.Cmd, error) {
	st, err := c.resolveStore()
	if err != nil {
		return nil, err
	}
	return c.storeCmdIn(st, args...), nil
}

func (c *Client) storeCmdIn(st store, args ...string) *exec.Cmd {
	cmd := exec.Command(c.GitBinary, append([]string{"--git-dir", st.gitDir}, args...)...)
	cmd.Dir = st.workTree
	return cmd
}

// pushSource returns the object side of a refspec, or "" for a delete refspec,
// which has none.
func pushSource(refspec string) string {
	src, _, _ := strings.Cut(refspec, ":")
	return strings.TrimPrefix(src, "+")
}

func withPathspec(args []string, pathspec []string) []string {
	if len(pathspec) == 0 {
		return args
	}
	args = append(args, "--")
	return append(args, pathspec...)
}

func (c *Client) diffNames(st store, base, head string, pathspec []string, extra ...string) ([]string, *vcstypes.PatchError) {
	args := append([]string{"diff", "-z", "--name-only"}, extra...)
	args = append(args, base, head)

	out, err := c.storeCmdIn(st, withPathspec(args, pathspec)...).Output()
	if err != nil {
		return nil, vcstypes.NewPatchError("diff_name_only", "git diff --name-only", err, "")
	}

	return vcstypes.SplitNULPaths(out), nil
}

func (c *Client) lfsFilesForPaths(st store, files []string) (vcstypes.LFSChangedFilesMetadata, *vcstypes.PatchError) {
	return vcstypes.LFSFilesForPaths(files, func(args ...string) (*exec.Cmd, error) {
		// An explicit --git-dir stops git from inferring the work tree, so
		// check-attr needs to be pointed at it to find .gitattributes.
		return c.storeCmdIn(st, append([]string{"--work-tree", st.workTree}, args...)...), nil
	})
}

func (c *Client) generatePatchData(pathspec []string) (vcstypes.PatchResult, *vcstypes.PatchError) {
	base, err := c.GetCommit()
	if base == "" || err != nil {
		return vcstypes.PatchResult{}, nil
	}

	head, err := c.GetHeadCommit()
	if err != nil || head == "" {
		return vcstypes.PatchResult{}, nil
	}

	st, err := c.resolveStore()
	if err != nil {
		return vcstypes.PatchResult{}, vcstypes.NewPatchError("diff_name_only", "git diff --name-only", err, "")
	}

	files, patchErr := c.diffNames(st, base, head, pathspec)
	if patchErr != nil {
		return vcstypes.PatchResult{}, patchErr
	}

	lfsChanged, patchErr := c.lfsFilesForPaths(st, files)
	if patchErr != nil {
		return vcstypes.PatchResult{}, patchErr
	}

	if lfsChanged.Count > 0 {
		return vcstypes.PatchResult{SHA: base, LFS: lfsChanged, OK: true}, nil
	}

	// jj tracks the whole working copy, so there is no untracked set to report.
	// The files added between base and @ are the closest equivalent: they are
	// the paths the patch creates, which is what callers use the field for.
	added, patchErr := c.diffNames(st, base, head, pathspec, "--diff-filter=A")
	if patchErr != nil {
		return vcstypes.PatchResult{}, patchErr
	}

	patchArgs := []string{"diff", base, head, "-p", "--binary"}
	patch, err := c.storeCmdIn(st, withPathspec(patchArgs, pathspec)...).Output()
	if err != nil {
		return vcstypes.PatchResult{}, vcstypes.NewPatchError("diff_patch", "git diff -p --binary", err, "")
	}

	return vcstypes.PatchResult{
		Patch: patch,
		SHA:   base,
		Untracked: vcstypes.UntrackedFilesMetadata{
			Files: added,
			Count: len(added),
		},
		OK: true,
	}, nil
}

func (c *Client) GeneratePatchFile(destDir string, pathspec []string) (vcstypes.PatchFile, error) {
	data, patchErr := c.generatePatchData(pathspec)
	if patchErr != nil {
		return vcstypes.PatchFile{}, patchErr
	}
	if !data.OK {
		return vcstypes.PatchFile{}, fmt.Errorf("unable to generate patch data")
	}

	if data.LFS.Count > 0 {
		return vcstypes.PatchFile{LFSChangedFiles: data.LFS}, nil
	}

	if len(data.Patch) == 0 {
		return vcstypes.PatchFile{UntrackedFiles: data.Untracked}, nil
	}

	outputPath := filepath.Join(destDir, data.SHA)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return vcstypes.PatchFile{}, fmt.Errorf("unable to create patch directory: %w", err)
	}

	if err := os.WriteFile(outputPath, data.Patch, 0644); err != nil {
		return vcstypes.PatchFile{}, fmt.Errorf("unable to write patch file: %w", err)
	}

	return vcstypes.PatchFile{
		Written:        true,
		Path:           outputPath,
		UntrackedFiles: data.Untracked,
	}, nil
}

func (c *Client) GeneratePatch(pathspec []string) ([]byte, *vcstypes.LFSChangedFilesMetadata, error) {
	data, patchErr := c.generatePatchData(pathspec)
	if patchErr != nil || !data.OK {
		return nil, nil, nil
	}

	if data.LFS.Count > 0 {
		return nil, &data.LFS, nil
	}

	if len(data.Patch) == 0 {
		return nil, nil, nil
	}

	return data.Patch, nil, nil
}

// GenerateDirtyPatches has no dirty state to report: every edit is snapshotted
// into @, so uncommitted work travels to the sandbox inside the commit PushRef
// sends rather than as a patch applied on top of it. The GetHeadCommit call is
// the guard — it fails closed on a repository whose working copy cannot be
// resolved, rather than silently reporting a clean tree.
func (c *Client) GenerateDirtyPatches() (vcstypes.DirtyPatches, error) {
	if _, err := c.GetHeadCommit(); err != nil {
		return vcstypes.DirtyPatches{}, err
	}

	return vcstypes.DirtyPatches{}, nil
}

func (c *Client) HasCommit(sha string) bool {
	if sha == "" {
		return false
	}

	cmd, err := c.storeCmd("cat-file", "-e", sha+"^{commit}")
	if err != nil {
		return false
	}

	return cmd.Run() == nil
}

// IsAncestor reports whether candidateSHA is an ancestor of (or equal to)
// headRef. Both are resolved as revsets, so a jj change id prefix works here as
// well as a commit id — which is what lets a sandbox session keyed on
// GetShortHead still match after the working copy has been rewritten.
func (c *Client) IsAncestor(candidateSHA, headRef string) bool {
	if candidateSHA == "" || headRef == "" {
		return false
	}

	revset := fmt.Sprintf("(%s) & ::(%s)", toRevset(candidateSHA), toRevset(headRef))
	out, _, err := c.resolveStale(revset, commitIDTemplate)
	if err != nil {
		return false
	}

	return firstLine(out) != ""
}

func (c *Client) hasConflict(rev string) bool {
	out, _, err := c.resolveStale(toRevset(rev), `conflict ++ "\n"`)
	if err != nil {
		return false
	}
	return firstLine(out) == "true"
}

func (c *Client) PushRef(opts vcstypes.PushRefOptions) error {
	if opts.Remote == "" {
		return fmt.Errorf("no remote provided")
	}
	if opts.Refspec == "" {
		return fmt.Errorf("no refspec provided")
	}

	st, err := c.resolveStore()
	if err != nil {
		return err
	}

	if src := pushSource(opts.Refspec); src != "" {
		// A conflicted change reaches the git store with one side of the conflict
		// as its content, so pushing it would send files the developer never wrote.
		if c.hasConflict(src) {
			return fmt.Errorf("cannot push %s: the jj working copy has unresolved conflicts, which do not survive the export to git; run `jj resolve` first", src)
		}

		// jj only writes @ to the store when it snapshots, so a refspec built from
		// a stale read can name a commit that is not there yet.
		if err := c.storeCmdIn(st, "cat-file", "-e", src+"^{commit}").Run(); err != nil {
			return fmt.Errorf("cannot push %s: the jj working copy has not been snapshotted into the git store", src)
		}
	}

	cmd := c.storeCmdIn(st, "push", opts.Remote, opts.Refspec)
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

func (c *Client) ApplyPatch(patch []byte) *exec.Cmd {
	return c.applyPatchCmd(patch, "apply", "--allow-empty", "-")
}

func (c *Client) ApplyPatchReject(patch []byte) *exec.Cmd {
	return c.applyPatchCmd(patch, "apply", "--reject", "--allow-empty", "-")
}

// applyPatchCmd points git at the backing store. A non-colocated workspace has
// no .git of its own, and without one git apply ignores .gitattributes.
func (c *Client) applyPatchCmd(patch []byte, args ...string) *exec.Cmd {
	var cmd *exec.Cmd
	if st, err := c.resolveStore(); err == nil {
		cmd = exec.Command(c.GitBinary, append([]string{"--git-dir", st.gitDir, "--work-tree", st.workTree}, args...)...)
		cmd.Dir = st.workTree
	} else {
		cmd = exec.Command(c.GitBinary, args...)
		cmd.Dir = c.applyDir()
	}

	cmd.Stdin = bytes.NewReader(patch)
	return cmd
}
