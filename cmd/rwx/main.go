package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rwx-cloud/rwx/internal/cli"
	internalerrors "github.com/rwx-cloud/rwx/internal/errors"
	"github.com/spf13/pflag"
)

// A HandledError has already been handled in the called function,
// but should return a non-zero exit code.
var HandledError = cli.HandledError

func main() {
	start := time.Now()
	err := rootCmd.Execute()

	recordTelemetry(err, start)

	if err == nil {
		return
	}

	var exitErr *cli.ExitCodeError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.Code)
	}

	if !errors.Is(err, HandledError) {
		if Debug {
			// Enabling debug output will print stacktraces
			fmt.Fprintf(os.Stderr, "Error: %+v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		}
	}

	os.Exit(1)
}

func recordTelemetry(err error, start time.Time) {
	if telem == nil {
		return
	}

	cmd, _, _ := rootCmd.Find(os.Args[1:])

	commandName := "rwx"
	if cmd != nil {
		commandName = cmd.CommandPath()
	}
	// Normalize "rwx <sub>" to just the subcommand path (e.g. "sandbox exec")
	commandName = strings.TrimPrefix(commandName, "rwx ")

	var flagNames []string
	if cmd != nil {
		cmd.Flags().Visit(func(f *pflag.Flag) {
			flagNames = append(flagNames, f.Name)
		})
	}

	telem.Record("cli.command", map[string]any{
		"command":       commandName,
		"flags":         flagNames,
		"output_format": Output,
		"duration_ms":   time.Since(start).Milliseconds(),
		"success":       err == nil,
	})

	if err != nil {
		telem.Record("cli.error", map[string]any{
			"command":    commandName,
			"flags":      flagNames,
			"error_type": classifyError(err),
			"handled":    errors.Is(err, HandledError),
		})
	}

	telem.Flush()
}

func classifyError(err error) string {
	switch {
	case errors.Is(err, internalerrors.ErrBadRequest):
		return "bad_request"
	case errors.Is(err, internalerrors.ErrUnauthenticated):
		return "unauthenticated"
	case errors.Is(err, internalerrors.ErrNotFound):
		return "not_found"
	case errors.Is(err, internalerrors.ErrFileNotExists):
		return "file_not_found"
	case errors.Is(err, internalerrors.ErrGone):
		return "gone"
	case errors.Is(err, internalerrors.ErrInternalServerError):
		return "internal_server_error"
	case errors.Is(err, internalerrors.ErrSSH):
		return "ssh_failed"
	case errors.Is(err, internalerrors.ErrPatch):
		return "patch_failed"
	case errors.Is(err, internalerrors.ErrTimeout):
		return "timeout"
	case errors.Is(err, internalerrors.ErrLSP):
		return "lsp_error"
	case errors.Is(err, internalerrors.ErrAmbiguousTaskKey):
		return "ambiguous_task_key"
	case errors.Is(err, internalerrors.ErrAmbiguousDefinitionPath):
		return "ambiguous_definition_path"
	case errors.Is(err, internalerrors.ErrNetworkTransient):
		return "network_transient_error"
	case errors.Is(err, internalerrors.ErrSandboxSetupFailure):
		return "sandbox_setup_failure"
	case errors.Is(err, internalerrors.ErrSandboxNoGitDir):
		return "sandbox_no_git_dir"
	default:
		return "unknown"
	}
}
