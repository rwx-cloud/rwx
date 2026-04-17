package errors

import (
	stderrors "errors"
	"fmt"
	"os"
	"runtime"

	"github.com/pkg/errors"
)

// sentinelError wraps an inner error while also matching a sentinel via errors.Is.
// It captures a stack trace at creation so %+v output includes caller context.
type sentinelError struct {
	inner    error
	sentinel error
	stack    []uintptr
}

func (e *sentinelError) Error() string   { return e.inner.Error() }
func (e *sentinelError) Unwrap() []error { return []error{e.inner, e.sentinel} }

// Format implements fmt.Formatter so that %+v renders the error with a stack trace.
func (e *sentinelError) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		fmt.Fprint(s, e.Error())
		if s.Flag('+') {
			frames := runtime.CallersFrames(e.stack)
			for {
				frame, more := frames.Next()
				fmt.Fprintf(s, "\n%s\n\t%s:%d", frame.Function, frame.File, frame.Line)
				if !more {
					break
				}
			}
		}
	case 's':
		fmt.Fprint(s, e.Error())
	case 'q':
		fmt.Fprintf(s, "%q", e.Error())
	}
}

// wrapSentinel wraps err so that errors.Is(result, sentinel) returns true.
// It captures a stack trace from the caller for %+v rendering.
func wrapSentinel(err, sentinel error) error {
	if err == nil {
		return nil
	}
	if stderrors.Is(err, sentinel) {
		return err
	}
	var pcs [32]uintptr
	// Skip runtime.Callers and wrapSentinel itself
	n := runtime.Callers(2, pcs[:])
	return &sentinelError{inner: err, sentinel: sentinel, stack: pcs[:n]}
}

var (
	ErrFileNotExists           = os.ErrNotExist
	ErrBadRequest              = errors.New("bad request")
	ErrNotFound                = errors.New("not found")
	ErrGone                    = errors.New("gone")
	ErrRetry                   = errors.New("retry")
	ErrSandboxNoGitDir         = errors.New("no .git directory found in sandbox. Set 'preserve-git-dir: true' on your git/clone task")
	ErrSandboxSetupFailure     = errors.New("sandbox setup failure")
	ErrSSH                     = errors.New("ssh error")
	ErrPatch                   = errors.New("patch error")
	ErrTimeout                 = errors.New("timeout")
	ErrLSP                     = errors.New("lsp error")
	ErrAmbiguousTaskKey        = errors.New("ambiguous task key")
	ErrAmbiguousDefinitionPath = errors.New("ambiguous definition path")
	ErrNetworkTransient        = errors.New("network transient error")

	// WrapSentinel wraps an error so that errors.Is returns true for the sentinel.
	WrapSentinel = wrapSentinel

	As        = errors.As
	Errorf    = errors.Errorf
	Is        = errors.Is
	New       = errors.New
	WithStack = errors.WithStack
	Wrap      = errors.Wrap
	Wrapf     = errors.Wrapf
)
