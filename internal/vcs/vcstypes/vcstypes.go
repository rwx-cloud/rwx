package vcstypes

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const defaultRemote = "origin"

func ConfiguredRemote() string {
	if remote := os.Getenv("RWX_GIT_REMOTE"); remote != "" {
		return remote
	}
	return defaultRemote
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

// SplitNULPaths splits NUL-delimited command output (git's -z form) into paths,
// dropping empty entries.
func SplitNULPaths(output []byte) []string {
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

// CommandFactory builds a command that can query a repository. Backends differ
// in how they reach the object store, so the shared helpers below take a
// factory instead of building commands themselves.
type CommandFactory func(args ...string) (*exec.Cmd, error)

// LFSFilesForPaths returns the subset of files that git-lfs is configured to
// filter. Callers must pass a factory whose commands already resolve to the
// right store and work tree; it is invoked once per file, so any repository
// lookups it needs should be resolved before the call.
func LFSFilesForPaths(files []string, newCmd CommandFactory) (LFSChangedFilesMetadata, *PatchError) {
	lfsChangedFiles := []string{}

	for _, file := range files {
		if file == "" {
			continue
		}

		cmd, err := newCmd("check-attr", "filter", "--", file)
		if err != nil {
			return LFSChangedFilesMetadata{}, NewPatchError("check_attr", "git check-attr filter", err, "")
		}

		// CombinedOutput mixes stderr into attrs, so pass it as the fallback
		// stderr for the PatchError (the *exec.ExitError won't carry .Stderr).
		attrs, err := cmd.CombinedOutput()
		if err != nil {
			return LFSChangedFilesMetadata{}, NewPatchError("check_attr", "git check-attr filter", err, string(attrs))
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

// PatchResult holds the patch data a backend generated for the working tree
// relative to its base commit on the remote. OK distinguishes "generated
// nothing because there is nothing to patch" from "generated an empty patch".
type PatchResult struct {
	Patch     []byte
	SHA       string
	Untracked UntrackedFilesMetadata
	LFS       LFSChangedFilesMetadata
	OK        bool
}

// UntrackedFilesMetadata carries the files a patch adds that do not exist in
// the base commit. Under git these are literally untracked working-tree files;
// a backend that tracks the whole working copy reports the files the patch adds
// relative to the base instead.
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

// PatchError identifies which command failed while generating a patch, along
// with its exit code and stderr, so callers can show the underlying error to
// the user and record a stable, PII-free bucket in telemetry.
type PatchError struct {
	Command  string // stable identifier for telemetry: diff_name_only, check_attr, ls_files, diff_patch
	Display  string // human-readable command, e.g. "git diff --name-only"
	Stderr   string // trimmed stderr (the user's own repo data)
	ExitCode int    // process exit code, or -1 if the command never started
}

func (e *PatchError) Error() string {
	if e.Stderr == "" {
		return fmt.Sprintf("failed to generate patch (%s)", e.Display)
	}
	return fmt.Sprintf("failed to generate patch (%s): %s", e.Display, e.Stderr)
}

// Reason buckets the stderr into a stable category for telemetry. Raw stderr
// must never be sent to telemetry — it embeds customer file paths, branch
// names, and repo layout.
func (e *PatchError) Reason() string {
	return classifyPatchStderr(e.Stderr)
}

// PatchFailureReason buckets any patch-related error into a stable, PII-free
// category for telemetry. It prefers a *PatchError's structured stderr and
// otherwise scans the error message for git-apply and LFS signatures. Only the
// returned bucket is safe to record — never the underlying message.
func PatchFailureReason(err error) string {
	if err == nil {
		return ""
	}
	var pe *PatchError
	if errors.As(err, &pe) {
		return pe.Reason()
	}
	return classifyPatchStderr(err.Error())
}

func classifyPatchStderr(s string) string {
	s = strings.ToLower(s)
	switch {
	case strings.Contains(s, "bad object"), strings.Contains(s, "shallow"):
		return "shallow_clone"
	case strings.Contains(s, "beyond a symbolic link"):
		return "beyond_symlink"
	case strings.Contains(s, "external filter"), strings.Contains(s, "filter-process"):
		return "missing_external_filter"
	case strings.Contains(s, "signal: killed"), strings.Contains(s, "out of memory"), strings.Contains(s, "cannot allocate memory"):
		return "oom_killed"
	case strings.Contains(s, "lfs file(s) changed locally"):
		return "lfs_changed"
	case strings.Contains(s, "already exists in working directory"):
		return "already_exists"
	case strings.Contains(s, "corrupt patch"):
		return "corrupt_patch"
	case strings.Contains(s, "does not apply"), strings.Contains(s, "patch failed"), strings.Contains(s, "partially applied"):
		return "patch_conflict"
	default:
		return "unknown"
	}
}

// NewPatchError builds a PatchError from a failed exec, extracting the exit
// code and stderr from an *exec.ExitError when available. fallbackStderr is
// used when the error doesn't carry captured stderr (e.g. CombinedOutput).
func NewPatchError(command, display string, err error, fallbackStderr string) *PatchError {
	pe := &PatchError{Command: command, Display: display, ExitCode: -1}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
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
