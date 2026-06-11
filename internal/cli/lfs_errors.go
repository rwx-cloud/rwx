package cli

import (
	"fmt"

	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/rwx-cloud/rwx/internal/git"
)

const sandboxLFSRecovery = "To recover, push your changes and reset the sandbox."

func runPatchLFSChangesError(changed git.LFSChangedFilesMetadata) error {
	return lfsChangesError(changed, "cannot be included in the RWX run patch", "To recover, commit and push your changes, then retry rwx run.")
}

func sandboxLFSChangesError(changed git.LFSChangedFilesMetadata) error {
	return lfsChangesError(changed, "cannot be synced to the sandbox", sandboxLFSRecovery)
}

func lfsChangesError(changed git.LFSChangedFilesMetadata, action, recovery string) error {
	count := changed.Count
	if count == 0 {
		count = len(changed.Files)
	}

	return errors.WrapSentinel(
		fmt.Errorf("%d LFS file(s) changed locally and %s:\n%s\n\n%s", count, action, indentLines(changed.Files), recovery),
		errors.ErrPatch,
	)
}
