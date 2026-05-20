// Package throttle provides a cross-process semaphore that caps how many `rwx`
// invocations can hold a given resource at once. It is intended to apply
// natural backpressure when many short-lived CLI processes are spawned in
// parallel (typically by coding agents or scripts) and would otherwise hammer
// the RWX API.
package throttle

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/rwx-cloud/rwx/internal/errors"
)

const (
	DefaultMaxConcurrency = 4
	DefaultWaitTimeout    = 5 * time.Minute
	pollInterval          = 100 * time.Millisecond
)

// Slot is an acquired throttle slot. Call Release when the throttled work is
// done. The OS releases the underlying file lock on process exit, so a missed
// Release does not strand a slot indefinitely.
type Slot struct {
	lock *flock.Flock
}

// Release frees the slot. Safe to call on a nil Slot.
func (s *Slot) Release() {
	if s == nil || s.lock == nil {
		return
	}
	_ = s.lock.Unlock()
}

// Config controls Acquire's behavior. Zero values use sensible defaults.
type Config struct {
	// LockDir is the directory holding the slot files. Defaults to
	// ~/.config/rwx/locks.
	LockDir string
	// MaxConcurrency is the number of slots. Defaults to DefaultMaxConcurrency.
	MaxConcurrency int
	// WaitTimeout caps how long Acquire blocks. Defaults to DefaultWaitTimeout.
	// On timeout, Acquire returns a nil Slot and TimedOut=true so the caller can
	// proceed unthrottled rather than failing the user's command.
	WaitTimeout time.Duration
	// Stderr receives the "waiting for slot…" message. Defaults to os.Stderr.
	Stderr io.Writer
}

// Result describes the outcome of an Acquire call.
type Result struct {
	// Slot is the acquired slot, or nil if the caller should proceed unthrottled
	// (TimedOut).
	Slot *Slot
	// WaitDuration is how long Acquire blocked before returning.
	WaitDuration time.Duration
	// Waited is true if Acquire had to block (vs. acquiring on the first try).
	Waited bool
	// TimedOut is true if Acquire gave up because WaitTimeout elapsed.
	TimedOut bool
}

// MaxConcurrencyFromEnv reads RWX_RESULTS_MAX_CONCURRENCY. Returns 0 if unset
// or invalid; callers should fall back to DefaultMaxConcurrency.
func MaxConcurrencyFromEnv() int {
	raw := strings.TrimSpace(os.Getenv("RWX_RESULTS_MAX_CONCURRENCY"))
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0
	}
	return n
}

// DefaultLockDir is ~/.config/rwx/locks.
func DefaultLockDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.Wrap(err, "unable to resolve home directory")
	}
	return filepath.Join(home, ".config", "rwx", "locks"), nil
}

// Acquire blocks until a slot is acquired, the wait times out, or ctx is
// cancelled. `prefix` names the resource being throttled (e.g. "results") and
// becomes the slot-file prefix on disk.
//
// On timeout, Result.Slot is nil and Result.TimedOut is true so the caller can
// proceed unthrottled — a stalled throttle should never block real work.
func Acquire(ctx context.Context, prefix string, cfg Config) (Result, error) {
	if cfg.MaxConcurrency <= 0 {
		cfg.MaxConcurrency = DefaultMaxConcurrency
	}
	if cfg.WaitTimeout <= 0 {
		cfg.WaitTimeout = DefaultWaitTimeout
	}
	if cfg.Stderr == nil {
		cfg.Stderr = os.Stderr
	}
	if cfg.LockDir == "" {
		dir, err := DefaultLockDir()
		if err != nil {
			return Result{}, err
		}
		cfg.LockDir = dir
	}

	if err := os.MkdirAll(cfg.LockDir, 0o755); err != nil {
		return Result{}, errors.Wrapf(err, "unable to create lock directory %q", cfg.LockDir)
	}

	locks := make([]*flock.Flock, cfg.MaxConcurrency)
	for i := range locks {
		path := filepath.Join(cfg.LockDir, fmt.Sprintf("%s-%d.lock", prefix, i))
		locks[i] = flock.New(path)
	}

	start := time.Now()

	if slot := tryAcquireAny(locks); slot != nil {
		return Result{Slot: slot}, nil
	}

	fmt.Fprintf(cfg.Stderr,
		"Waiting for an `rwx %s` slot (concurrency limit: %d). "+
			"Set RWX_RESULTS_MAX_CONCURRENCY to raise the cap.\n",
		prefix, cfg.MaxConcurrency)

	deadline := start.Add(cfg.WaitTimeout)
	for {
		if slot := tryAcquireAny(locks); slot != nil {
			return Result{Slot: slot, WaitDuration: time.Since(start), Waited: true}, nil
		}

		if time.Now().After(deadline) {
			fmt.Fprintf(cfg.Stderr,
				"Gave up waiting for an `rwx %s` slot after %s; proceeding without throttle.\n",
				prefix, cfg.WaitTimeout)
			return Result{WaitDuration: time.Since(start), Waited: true, TimedOut: true}, nil
		}

		select {
		case <-ctx.Done():
			return Result{WaitDuration: time.Since(start), Waited: true}, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func tryAcquireAny(locks []*flock.Flock) *Slot {
	for _, lock := range locks {
		locked, err := lock.TryLock()
		if err == nil && locked {
			return &Slot{lock: lock}
		}
	}
	return nil
}
