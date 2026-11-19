package cli

import (
	"fmt"

	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/rwx-cloud/rwx/internal/vcs"
	"github.com/rwx-cloud/rwx/internal/vcs/vcstypes"
)

const sandboxLFSRecovery = "To recover, push your changes and reset the sandbox."

func runPatchLFSChangesError(changed vcs.LFSChangedFilesMetadata, source vcstypes.PatchSource) error {
	recovery := "To recover, commit any uncommitted LFS changes and push, then retry rwx run."
	if source == vcstypes.Unpushed {
		recovery = "To recover, push your local commits, then retry rwx run."
	}
	return lfsChangesError(changed, "cannot be included in the RWX run patch", recovery)
}

func sandboxLFSChangesError(changed vcs.LFSChangedFilesMetadata) error {
	return lfsChangesError(changed, "cannot be synced to the sandbox", sandboxLFSRecovery)
}

func lfsChangesError(changed vcs.LFSChangedFilesMetadata, action, recovery string) error {
	count := changed.Count
	if count == 0 {
		count = len(changed.Files)
	}

	return errors.WrapSentinel(
		fmt.Errorf("%d LFS file(s) changed locally and %s:\n%s\n\n%s", count, action, indentLines(changed.Files), recovery),
		errors.ErrPatch,
	)
}
