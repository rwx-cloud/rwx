package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/rwx-cloud/rwx/internal/git"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

const (
	sandboxPrivateTestKey = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACDiyT6ht8Z2XBEJpLR4/xmNouq5KDdn5G++cUcTH4EhzwAAAJhIWxlBSFsZ
QQAAAAtzc2gtZWQyNTUxOQAAACDiyT6ht8Z2XBEJpLR4/xmNouq5KDdn5G++cUcTH4Ehzw
AAAEC6442PQKevgYgeT0SIu9zwlnEMl6MF59ZgM+i0ByMv4eLJPqG3xnZcEQmktHj/GY2i
6rkoN2fkb75xRxMfgSHPAAAAEG1pbnQgQ0xJIHRlc3RpbmcBAgMEBQ==
-----END OPENSSH PRIVATE KEY-----`
	sandboxPublicTestKey = `ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOLJPqG3xnZcEQmktHj/GY2i6rkoN2fkb75xRxMfgSHP rwx CLI testing`
)

func TestService_ListSandboxes(t *testing.T) {
	t.Run("returns list without error", func(t *testing.T) {
		setup := setupTest(t)

		setup.mockAPI.MockListSandboxRuns = func() (*api.ListSandboxRunsResult, error) {
			return &api.ListSandboxRunsResult{Runs: []api.SandboxRunSummary{}}, nil
		}

		result, err := setup.service.ListSandboxes(cli.ListSandboxesConfig{
			Json: true,
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Sandboxes)
	})
}

func TestService_LockWaitOutput(t *testing.T) {
	t.Run("no output when lock is uncontended", func(t *testing.T) {
		setup := setupTest(t)

		setup.mockAPI.MockListSandboxRuns = func() (*api.ListSandboxRunsResult, error) {
			return &api.ListSandboxRunsResult{Runs: []api.SandboxRunSummary{}}, nil
		}

		_, err := setup.service.ListSandboxes(cli.ListSandboxesConfig{Json: false})
		require.NoError(t, err)
		require.Empty(t, setup.mockStderr.String(), "expected no stderr output when lock is uncontended")
	})

	t.Run("shows waiting message when lock is contended", func(t *testing.T) {
		setup := setupTest(t)

		setup.mockAPI.MockListSandboxRuns = func() (*api.ListSandboxRunsResult, error) {
			return &api.ListSandboxRunsResult{Runs: []api.SandboxRunSummary{}}, nil
		}

		// Hold the lock to force contention
		lock, err := cli.LockSandboxStorage()
		require.NoError(t, err)

		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = setup.service.ListSandboxes(cli.ListSandboxesConfig{Json: false})
		}()

		// Wait for the spinner message to appear, with a timeout
		deadline := time.After(5 * time.Second)
		for {
			if strings.Contains(setup.mockStderr.String(), "Waiting for another sandbox operation") {
				break
			}
			select {
			case <-deadline:
				t.Fatal("timed out waiting for spinner message to appear on stderr")
			default:
				runtime.Gosched()
			}
		}

		cli.UnlockSandboxStorage(lock)
		<-done

		require.Contains(t, setup.mockStderr.String(), "Waiting for another sandbox operation to complete...")
	})

	t.Run("suppresses waiting message in json mode", func(t *testing.T) {
		setup := setupTest(t)

		setup.mockAPI.MockListSandboxRuns = func() (*api.ListSandboxRunsResult, error) {
			return &api.ListSandboxRunsResult{Runs: []api.SandboxRunSummary{}}, nil
		}

		// Hold the lock to force contention
		lock, err := cli.LockSandboxStorage()
		require.NoError(t, err)

		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = setup.service.ListSandboxes(cli.ListSandboxesConfig{Json: true})
		}()

		// Let the goroutine reach the blocking Lock() call, then release
		time.Sleep(100 * time.Millisecond)
		cli.UnlockSandboxStorage(lock)
		<-done

		require.Empty(t, setup.mockStderr.String(), "expected no stderr output in json mode")
	})
}

func TestService_ListSandboxes_BulkAPI(t *testing.T) {
	t.Run("uses bulk API to determine active sandboxes", func(t *testing.T) {
		setup := setupTest(t)
		seedSandboxStorageMulti(t, setup.tmp, map[string]cli.SandboxSession{
			"main:" + setup.absConfig(".rwx/sandbox.yml"): {
				RunID:       "run-active",
				ConfigFile:  setup.absConfig(".rwx/sandbox.yml"),
				ScopedToken: "token-active",
			},
			"main:" + setup.absConfig(".rwx/other.yml"): {
				RunID:       "run-expired",
				ConfigFile:  setup.absConfig(".rwx/other.yml"),
				ScopedToken: "token-expired",
			},
		})

		setup.mockAPI.MockListSandboxRuns = func() (*api.ListSandboxRunsResult, error) {
			return &api.ListSandboxRunsResult{
				Runs: []api.SandboxRunSummary{
					{ID: "run-active", RunURL: "https://cloud.rwx.com/runs/run-active"},
				},
			}, nil
		}

		// Fallback check confirms the expired run is gone
		setup.mockAPI.MockGetSandboxConnectionInfo = func(runID, scopedToken string) (api.SandboxConnectionInfo, error) {
			if runID == "run-expired" {
				return api.SandboxConnectionInfo{}, errors.ErrGone
			}
			return api.SandboxConnectionInfo{}, nil
		}

		result, err := setup.service.ListSandboxes(cli.ListSandboxesConfig{Json: true})

		require.NoError(t, err)
		require.Len(t, result.Sandboxes, 1)
		require.Equal(t, "run-active", result.Sandboxes[0].RunID)
		require.Equal(t, "active", result.Sandboxes[0].Status)

		// Verify expired was pruned from storage
		storage, err := cli.LoadSandboxStorage()
		require.NoError(t, err)
		require.Len(t, storage.Sandboxes, 1)
		_, exists := storage.Sandboxes["main:"+setup.absConfig(".rwx/sandbox.yml")]
		require.True(t, exists)
	})

	t.Run("merges remote-only runs into local storage", func(t *testing.T) {
		setup := setupTest(t)
		seedSandboxStorageMulti(t, setup.tmp, map[string]cli.SandboxSession{
			"main:" + setup.absConfig(".rwx/sandbox.yml"): {
				RunID:       "run-local",
				ConfigFile:  setup.absConfig(".rwx/sandbox.yml"),
				ScopedToken: "token-local",
			},
		})

		remoteCliState := cli.EncodeCliState("develop", setup.absConfig(".rwx/sandbox.yml"))
		setup.mockAPI.MockListSandboxRuns = func() (*api.ListSandboxRunsResult, error) {
			return &api.ListSandboxRunsResult{
				Runs: []api.SandboxRunSummary{
					{ID: "run-local", RunURL: "https://cloud.rwx.com/runs/run-local"},
					{ID: "run-remote", RunURL: "https://cloud.rwx.com/runs/run-remote", CliState: remoteCliState},
				},
			}, nil
		}

		result, err := setup.service.ListSandboxes(cli.ListSandboxesConfig{Json: true})

		require.NoError(t, err)
		require.Len(t, result.Sandboxes, 2)

		runIDs := make([]string, len(result.Sandboxes))
		for i, sb := range result.Sandboxes {
			runIDs[i] = sb.RunID
		}
		require.ElementsMatch(t, []string{"run-local", "run-remote"}, runIDs)

		// Verify remote run was saved to storage
		storage, err := cli.LoadSandboxStorage()
		require.NoError(t, err)
		require.Len(t, storage.Sandboxes, 2)
		session, found := storage.GetSession("develop", setup.absConfig(".rwx/sandbox.yml"))
		require.True(t, found)
		require.Equal(t, "run-remote", session.RunID)
		require.Equal(t, "https://cloud.rwx.com/runs/run-remote", session.RunURL)
	})

	t.Run("skips remote runs without cli_state", func(t *testing.T) {
		setup := setupTest(t)
		seedSandboxStorageMulti(t, setup.tmp, map[string]cli.SandboxSession{})

		setup.mockAPI.MockListSandboxRuns = func() (*api.ListSandboxRunsResult, error) {
			return &api.ListSandboxRunsResult{
				Runs: []api.SandboxRunSummary{
					{ID: "run-no-state", RunURL: "https://cloud.rwx.com/runs/run-no-state"},
				},
			}, nil
		}

		result, err := setup.service.ListSandboxes(cli.ListSandboxesConfig{Json: true})

		require.NoError(t, err)
		require.Empty(t, result.Sandboxes)
	})

	t.Run("does not overwrite existing local session with remote run", func(t *testing.T) {
		setup := setupTest(t)
		seedSandboxStorageMulti(t, setup.tmp, map[string]cli.SandboxSession{
			"main:" + setup.absConfig(".rwx/sandbox.yml"): {
				RunID:       "run-local",
				ConfigFile:  setup.absConfig(".rwx/sandbox.yml"),
				ScopedToken: "my-scoped-token",
			},
		})

		// Remote has a run with cli_state pointing to the same key but different run ID
		remoteCliState := cli.EncodeCliState("main", setup.absConfig(".rwx/sandbox.yml"))
		setup.mockAPI.MockListSandboxRuns = func() (*api.ListSandboxRunsResult, error) {
			return &api.ListSandboxRunsResult{
				Runs: []api.SandboxRunSummary{
					{ID: "run-local", RunURL: "https://cloud.rwx.com/runs/run-local"},
					{ID: "run-remote-new", RunURL: "https://cloud.rwx.com/runs/run-remote-new", CliState: remoteCliState},
				},
			}, nil
		}

		result, err := setup.service.ListSandboxes(cli.ListSandboxesConfig{Json: true})
		require.NoError(t, err)

		// Local session key is occupied by run-local, so run-remote-new should NOT overwrite it
		require.Len(t, result.Sandboxes, 1)
		require.Equal(t, "run-local", result.Sandboxes[0].RunID)

		// Verify the scoped token was preserved
		storage, err := cli.LoadSandboxStorage()
		require.NoError(t, err)
		session, found := storage.GetSession("main", setup.absConfig(".rwx/sandbox.yml"))
		require.True(t, found)
		require.Equal(t, "run-local", session.RunID)
		require.Equal(t, "my-scoped-token", session.ScopedToken)
	})
}

func TestService_ListSandboxes_PrunesExpired(t *testing.T) {
	t.Run("removes expired sandboxes from storage and results", func(t *testing.T) {
		setup := setupTest(t)
		seedSandboxStorageMulti(t, setup.tmp, map[string]cli.SandboxSession{
			"main:" + setup.absConfig(".rwx/sandbox.yml"): {
				RunID:       "run-active",
				ConfigFile:  setup.absConfig(".rwx/sandbox.yml"),
				ScopedToken: "token-active",
			},
			"main:" + setup.absConfig(".rwx/other.yml"): {
				RunID:       "run-expired",
				ConfigFile:  setup.absConfig(".rwx/other.yml"),
				ScopedToken: "token-expired",
			},
		})

		// Bulk API only returns the active run
		setup.mockAPI.MockListSandboxRuns = func() (*api.ListSandboxRunsResult, error) {
			return &api.ListSandboxRunsResult{
				Runs: []api.SandboxRunSummary{
					{ID: "run-active", RunURL: "https://cloud.rwx.com/runs/run-active"},
				},
			}, nil
		}

		// Fallback check confirms expired run is gone
		setup.mockAPI.MockGetSandboxConnectionInfo = func(runID, scopedToken string) (api.SandboxConnectionInfo, error) {
			if runID == "run-expired" {
				return api.SandboxConnectionInfo{}, errors.ErrGone
			}
			return api.SandboxConnectionInfo{}, nil
		}

		result, err := setup.service.ListSandboxes(cli.ListSandboxesConfig{
			Json: true,
		})

		require.NoError(t, err)
		require.Len(t, result.Sandboxes, 1)
		require.Equal(t, "run-active", result.Sandboxes[0].RunID)
		require.Equal(t, "active", result.Sandboxes[0].Status)

		// Verify storage was pruned
		storage, err := cli.LoadSandboxStorage()
		require.NoError(t, err)
		require.Len(t, storage.Sandboxes, 1)
		_, exists := storage.Sandboxes["main:"+setup.absConfig(".rwx/sandbox.yml")]
		require.True(t, exists)
	})

	t.Run("removes all local sandboxes not in API response when confirmed expired", func(t *testing.T) {
		setup := setupTest(t)
		seedSandboxStorageMulti(t, setup.tmp, map[string]cli.SandboxSession{
			"main:" + setup.absConfig(".rwx/active.yml"): {
				RunID:       "run-active",
				ConfigFile:  setup.absConfig(".rwx/active.yml"),
				ScopedToken: "token-active",
			},
			"main:" + setup.absConfig(".rwx/notfound.yml"): {
				RunID:       "run-notfound",
				ConfigFile:  setup.absConfig(".rwx/notfound.yml"),
				ScopedToken: "token-notfound",
			},
			"main:" + setup.absConfig(".rwx/gone.yml"): {
				RunID:       "run-gone",
				ConfigFile:  setup.absConfig(".rwx/gone.yml"),
				ScopedToken: "token-gone",
			},
		})

		// Only run-active is returned by the API
		setup.mockAPI.MockListSandboxRuns = func() (*api.ListSandboxRunsResult, error) {
			return &api.ListSandboxRunsResult{
				Runs: []api.SandboxRunSummary{
					{ID: "run-active", RunURL: "https://cloud.rwx.com/runs/run-active"},
				},
			}, nil
		}

		// Fallback checks confirm both missing runs are gone
		setup.mockAPI.MockGetSandboxConnectionInfo = func(runID, scopedToken string) (api.SandboxConnectionInfo, error) {
			switch runID {
			case "run-notfound":
				return api.SandboxConnectionInfo{}, errors.ErrNotFound
			case "run-gone":
				return api.SandboxConnectionInfo{}, errors.ErrGone
			}
			return api.SandboxConnectionInfo{}, nil
		}

		result, err := setup.service.ListSandboxes(cli.ListSandboxesConfig{
			Json: true,
		})

		require.NoError(t, err)
		require.Len(t, result.Sandboxes, 1)
		require.Equal(t, "run-active", result.Sandboxes[0].RunID)

		storage, err := cli.LoadSandboxStorage()
		require.NoError(t, err)
		require.Len(t, storage.Sandboxes, 1)
	})

	t.Run("keeps initializing sandbox not yet in bulk API response", func(t *testing.T) {
		setup := setupTest(t)
		seedSandboxStorageMulti(t, setup.tmp, map[string]cli.SandboxSession{
			"main:" + setup.absConfig(".rwx/sandbox.yml"): {
				RunID:       "run-initializing",
				ConfigFile:  setup.absConfig(".rwx/sandbox.yml"),
				ScopedToken: "token-init",
			},
		})

		// Bulk API does not return the initializing run
		setup.mockAPI.MockListSandboxRuns = func() (*api.ListSandboxRunsResult, error) {
			return &api.ListSandboxRunsResult{Runs: []api.SandboxRunSummary{}}, nil
		}

		// Fallback check shows the run is still alive
		setup.mockAPI.MockGetSandboxConnectionInfo = func(runID, scopedToken string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Polling: api.PollingResult{Completed: false},
			}, nil
		}

		result, err := setup.service.ListSandboxes(cli.ListSandboxesConfig{Json: true})

		require.NoError(t, err)
		require.Len(t, result.Sandboxes, 1)
		require.Equal(t, "run-initializing", result.Sandboxes[0].RunID)
		require.Equal(t, "active", result.Sandboxes[0].Status)

		// Verify it was NOT pruned from storage
		storage, err := cli.LoadSandboxStorage()
		require.NoError(t, err)
		require.Len(t, storage.Sandboxes, 1)
	})

	t.Run("prunes sandbox when fallback connection info returns an error", func(t *testing.T) {
		setup := setupTest(t)
		seedSandboxStorageMulti(t, setup.tmp, map[string]cli.SandboxSession{
			"main:" + setup.absConfig(".rwx/sandbox.yml"): {
				RunID:       "run-cancelled",
				ConfigFile:  setup.absConfig(".rwx/sandbox.yml"),
				ScopedToken: "token-cancel",
			},
		})

		setup.mockAPI.MockListSandboxRuns = func() (*api.ListSandboxRunsResult, error) {
			return &api.ListSandboxRunsResult{Runs: []api.SandboxRunSummary{}}, nil
		}

		// Fallback returns an error (e.g. 400 "run has been cancelled")
		setup.mockAPI.MockGetSandboxConnectionInfo = func(runID, scopedToken string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{}, fmt.Errorf("run has been cancelled")
		}

		result, err := setup.service.ListSandboxes(cli.ListSandboxesConfig{Json: true})

		require.NoError(t, err)
		require.Empty(t, result.Sandboxes)

		storage, err := cli.LoadSandboxStorage()
		require.NoError(t, err)
		require.Empty(t, storage.Sandboxes)
	})
}

func seedSandboxStorageMulti(t *testing.T, tmpHome string, sessions map[string]cli.SandboxSession) {
	t.Helper()

	storageDir := filepath.Join(tmpHome, ".rwx", "sandboxes")
	require.NoError(t, os.MkdirAll(storageDir, 0o755))

	storage := cli.SandboxStorage{
		Version:   1,
		Sandboxes: sessions,
	}

	data, err := json.Marshal(storage)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(storageDir, "sandboxes.json"), data, 0o644))
}

func TestService_ExecSandbox(t *testing.T) {
	t.Run("executes command in sandbox successfully", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-123"
		address := "192.168.1.1:22"
		connectedViaSSH := false
		var executedCommands []string

		// Mock run status to indicate run is active
		setup.mockAPI.MockRunStatus = func(cfg api.RunStatusConfig) (api.RunStatusResult, error) {
			return api.RunStatusResult{
				RunID:   runID,
				Polling: api.PollingResult{Completed: false},
			}, nil
		}

		// Mock sandbox connection info
		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			require.Equal(t, runID, id)
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			require.Equal(t, address, addr)
			connectedViaSSH = true
			return nil
		}

		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			executedCommands = append(executedCommands, cmd)
			return 0, nil
		}

		// Pull mocks (no changes on sandbox)

		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return nil, nil, nil
		}

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
		})

		require.NoError(t, err)
		require.Equal(t, runID, result.RunID)
		require.Equal(t, 0, result.ExitCode)
		require.Equal(t, "", result.RunURL)
		require.True(t, connectedViaSSH)
		require.Contains(t, executedCommands, "echo hello")
	})

	t.Run("returns non-zero exit code from command", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-456"
		address := "192.168.1.1:22"
		userCommandRan := false

		setup.mockAPI.MockRunStatus = func(cfg api.RunStatusConfig) (api.RunStatusResult, error) {
			return api.RunStatusResult{
				RunID:   runID,
				Polling: api.PollingResult{Completed: false},
			}, nil
		}

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			if cmd == "false" {
				userCommandRan = true
				return 1, nil // Non-zero exit code
			}
			return 0, nil // sync markers
		}

		// Pull mocks (no changes on sandbox)

		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return nil, nil, nil
		}

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"false"},
			RunID:      runID,
			Json:       true,
		})

		require.NoError(t, err)
		require.Equal(t, runID, result.RunID)
		require.Equal(t, 1, result.ExitCode)
		require.True(t, userCommandRan)
	})

	t.Run("shell-quotes command arguments for remote execution", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-quote-123"
		address := "192.168.1.1:22"
		var executedCommands []string

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			executedCommands = append(executedCommands, cmd)
			return 0, nil
		}

		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return nil, nil, nil
		}

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"bash", "-c", "cat README.md"},
			RunID:      runID,
			Json:       true,
		})

		require.NoError(t, err)
		require.Equal(t, 0, result.ExitCode)
		require.Contains(t, executedCommands, "bash -c 'cat README.md'")
	})

	t.Run("returns error when run is no longer active", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-expired"

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable: false,
				Polling:     api.PollingResult{Completed: true}, // Run has ended
			}, nil
		}

		_, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "completed before becoming ready")
	})
}

func TestService_ExecSandbox_Sync(t *testing.T) {
	t.Run("syncs changes when Sync is true", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-sync-123"
		address := "192.168.1.1:22"
		patchApplied := false
		var appliedPatch []byte

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return []byte("diff --git a/file.txt b/file.txt\n"), nil, nil
		}

		setup.mockSSH.MockExecuteCommandWithOutput = func(command string) (int, string, error) {
			// Return empty for git diff --name-only and ls-files (no dirty files on sandbox)
			return 0, "", nil
		}

		setup.mockSSH.MockExecuteCommandWithStdinAndCombinedOutput = func(command string, stdin io.Reader) (int, string, error) {
			require.Equal(t, "/usr/bin/git apply --allow-empty -", command)
			appliedPatch, _ = io.ReadAll(stdin)
			patchApplied = true
			return 0, "", nil
		}

		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			return 0, nil
		}

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
			Sync:       true,
		})

		require.NoError(t, err)
		require.Equal(t, runID, result.RunID)
		require.Equal(t, 0, result.ExitCode)
		require.True(t, patchApplied)
		require.Equal(t, "diff --git a/file.txt b/file.txt\n", string(appliedPatch))
	})

	t.Run("skips sync when Sync is false", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-no-sync-123"
		address := "192.168.1.1:22"
		syncPatchApplied := false

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		setup.mockSSH.MockExecuteCommandWithStdin = func(command string, stdin io.Reader) (int, error) {
			syncPatchApplied = true
			return 0, nil
		}

		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			return 0, nil
		}

		// Pull mocks (no changes on sandbox)

		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return nil, nil, nil
		}

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
			Sync:       false,
		})

		require.NoError(t, err)
		require.Equal(t, runID, result.RunID)
		require.Equal(t, 0, result.ExitCode)
		require.False(t, syncPatchApplied)
	})

	t.Run("skips sync when no changes", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-no-changes-123"
		address := "192.168.1.1:22"
		syncPatchApplied := false

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return nil, nil, nil // No changes
		}

		setup.mockSSH.MockExecuteCommandWithOutput = func(command string) (int, string, error) {
			return 0, "", nil
		}

		setup.mockSSH.MockExecuteCommandWithStdin = func(command string, stdin io.Reader) (int, error) {
			syncPatchApplied = true
			return 0, nil
		}

		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			return 0, nil
		}

		// Pull mocks (no changes on sandbox)

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
			Sync:       true,
		})

		require.NoError(t, err)
		require.Equal(t, runID, result.RunID)
		require.Equal(t, 0, result.ExitCode)
		require.False(t, syncPatchApplied)
	})

	t.Run("creates sync ref even when no local changes", func(t *testing.T) {
		// When there are no local changes, sync returns early but must still
		// create refs/rwx-sync so pull has a valid baseline to diff against.
		setup := setupTest(t)

		runID := "run-no-changes-ref"
		address := "192.168.1.1:22"
		createdSyncRef := false

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return nil, nil, nil // No changes
		}

		setup.mockSSH.MockExecuteCommandWithOutput = func(cmd string) (int, string, error) {
			return 0, "", nil
		}

		var commandOrder []string
		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			commandOrder = append(commandOrder, cmd)
			if strings.Contains(cmd, "update-ref refs/rwx-sync HEAD") {
				createdSyncRef = true
			}
			return 0, nil
		}

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
			Sync:       true,
		})

		require.NoError(t, err)
		require.Equal(t, 0, result.ExitCode)
		require.True(t, createdSyncRef, "sync should create refs/rwx-sync even with no local changes")

		// The update-ref command must be wrapped in sync markers so it doesn't appear in task logs.
		for i, cmd := range commandOrder {
			if strings.Contains(cmd, "update-ref refs/rwx-sync HEAD") && !strings.Contains(cmd, "update-ref -d") {
				require.Greater(t, i, 0, "update-ref should not be first command")
				require.Equal(t, "__rwx_sandbox_sync_start__", commandOrder[i-1],
					"update-ref should be preceded by sync start marker")
				require.Less(t, i, len(commandOrder)-1, "update-ref should not be last command")
				require.Equal(t, "__rwx_sandbox_sync_end__", commandOrder[i+1],
					"update-ref should be followed by sync end marker")
			}
		}
	})

	t.Run("warns and skips sync for LFS files", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-lfs-123"
		address := "192.168.1.1:22"
		syncPatchApplied := false
		generatePatchCallCount := 0

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			generatePatchCallCount++
			if generatePatchCallCount == 1 {
				// First call: sync phase - return LFS metadata
				return nil, &git.LFSChangedFilesMetadata{Files: []string{"large.bin"}, Count: 1}, nil
			}
			// Second call: pull phase - no local changes
			return nil, nil, nil
		}

		setup.mockSSH.MockExecuteCommandWithStdin = func(command string, stdin io.Reader) (int, error) {
			syncPatchApplied = true
			return 0, nil
		}

		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			return 0, nil
		}

		// Pull mocks (no changes on sandbox)

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       false, // Enable warning output
			Sync:       true,
		})

		require.NoError(t, err)
		require.Equal(t, runID, result.RunID)
		require.Equal(t, 0, result.ExitCode)
		require.False(t, syncPatchApplied)
		require.Contains(t, setup.mockStderr.String(), "LFS file(s) changed")
	})

	t.Run("returns error when git apply fails", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-apply-fail-123"
		address := "192.168.1.1:22"

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return []byte("invalid patch"), nil, nil
		}

		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			return 0, nil
		}

		setup.mockSSH.MockExecuteCommandWithOutput = func(cmd string) (int, string, error) {
			return 0, "", nil
		}

		setup.mockSSH.MockExecuteCommandWithStdinAndCombinedOutput = func(command string, stdin io.Reader) (int, string, error) {
			return 1, "error: patch failed", nil // git apply failed
		}

		_, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
			Sync:       true,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "git apply failed")
	})

	t.Run("syncs changes and reverts sandbox after pull", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-sync-123"
		address := "192.168.1.1:22"
		var commandOrder []string
		var stdinCommandOrder []string

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return []byte("diff --git a/file.txt b/file.txt\n"), nil, nil
		}

		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			commandOrder = append(commandOrder, cmd)
			return 0, nil
		}

		setup.mockSSH.MockExecuteCommandWithOutput = func(cmd string) (int, string, error) {
			return 0, "", nil
		}

		setup.mockSSH.MockExecuteCommandWithStdinAndCombinedOutput = func(command string, stdin io.Reader) (int, string, error) {
			stdinCommandOrder = append(stdinCommandOrder, command)
			return 0, "", nil
		}

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
			Sync:       true,
		})

		require.NoError(t, err)
		require.Equal(t, runID, result.RunID)
		require.Equal(t, 0, result.ExitCode)
		require.Equal(t, "__rwx_sandbox_lock_requested__", commandOrder[0])
		require.Contains(t, commandOrder, "__rwx_sandbox_sync_start__")
		require.Contains(t, commandOrder, "echo hello")
		// git apply uses stdin method
		require.GreaterOrEqual(t, len(stdinCommandOrder), 1)
		require.Equal(t, "/usr/bin/git apply --allow-empty -", stdinCommandOrder[0])
		// Sandbox should be reverted after pull (git checkout + git clean)
		lastSyncEnd := -1
		for i, cmd := range commandOrder {
			if strings.Contains(cmd, "git checkout .") && strings.Contains(cmd, "git clean -fd") {
				lastSyncEnd = i
			}
		}
		require.NotEqual(t, -1, lastSyncEnd, "sandbox should be reverted after pull")

		// Snapshot and reset git commands must be wrapped in sync markers
		// so they don't appear in task logs. Find the sync block that contains them.
		snapshotIdx := -1
		resetIdx := -1
		for i, cmd := range commandOrder {
			if strings.Contains(cmd, "git update-ref -d refs/rwx-sync") {
				snapshotIdx = i
			}
			if strings.Contains(cmd, "git reset HEAD~1") {
				resetIdx = i
			}
		}
		require.NotEqual(t, -1, snapshotIdx, "snapshot command should be present")
		require.NotEqual(t, -1, resetIdx, "reset command should be present")

		// Walk backward from snapshot to find sync start
		foundStart := false
		for j := snapshotIdx - 1; j >= 0; j-- {
			if commandOrder[j] == "__rwx_sandbox_sync_start__" {
				foundStart = true
				break
			}
			if commandOrder[j] == "__rwx_sandbox_sync_end__" {
				break // Hit end of a different sync block
			}
		}
		require.True(t, foundStart, "snapshot/reset commands should be preceded by sync start marker")

		// Walk forward from reset to find sync end
		foundEnd := false
		for j := resetIdx + 1; j < len(commandOrder); j++ {
			if commandOrder[j] == "__rwx_sandbox_sync_end__" {
				foundEnd = true
				break
			}
			if commandOrder[j] == "__rwx_sandbox_sync_start__" {
				break // Hit start of a different sync block
			}
		}
		require.True(t, foundEnd, "snapshot/reset commands should be followed by sync end marker")
	})

	t.Run("returns helpful error when git is not installed", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-no-git-123"
		address := "192.168.1.1:22"

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return []byte("patch"), nil, nil
		}

		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			return 0, nil
		}

		setup.mockSSH.MockExecuteCommandWithOutput = func(cmd string) (int, string, error) {
			return 0, "", nil
		}

		setup.mockSSH.MockExecuteCommandWithStdinAndCombinedOutput = func(command string, stdin io.Reader) (int, string, error) {
			return 127, "", nil // command not found
		}

		_, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
			Sync:       true,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "git is not installed")
	})

	t.Run("stops sandbox and removes session when .git directory is missing", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-no-git-dir-123"
		address := "192.168.1.1:22"

		seedSandboxStorage(t, setup.tmp, runID, "scoped-token-no-git")

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return []byte("patch"), nil, nil
		}

		sandboxEndCalled := false
		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			if cmd == "test -d .git" {
				return 1, nil // .git directory does not exist
			}
			if cmd == "__rwx_sandbox_end__" {
				sandboxEndCalled = true
			}
			return 0, nil
		}

		_, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
			Sync:       true,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "no .git directory")
		require.Contains(t, err.Error(), "preserve-git-dir: true")
		require.True(t, sandboxEndCalled, "sandbox should be stopped when .git directory is missing")

		// Verify session was removed from storage
		storage, loadErr := cli.LoadSandboxStorage()
		require.NoError(t, loadErr)
		_, _, found := storage.FindByRunID(runID)
		require.False(t, found, "session should be removed from storage")
	})

}

func TestService_ExecSandbox_Pull(t *testing.T) {
	t.Run("pulls changes from sandbox after command execution", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-pull-123"
		address := "192.168.1.1:22"

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		// Sandbox has a change to file.txt
		sandboxPatch := "diff --git a/file.txt b/file.txt\nindex abc..def 100644\n--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+new\n"

		setup.mockSSH.MockExecuteCommandWithOutput = func(cmd string) (int, string, error) {
			if strings.Contains(cmd, "git diff refs/rwx-sync") {
				return 0, sandboxPatch, nil
			}
			return 0, "", nil
		}

		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			return 0, nil
		}

		// No local changes
		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return nil, nil, nil
		}

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
			Sync:       true,
		})

		require.NoError(t, err)
		require.Equal(t, runID, result.RunID)
		require.Equal(t, 0, result.ExitCode)
		require.Contains(t, result.PulledFiles, "file.txt")
	})

	t.Run("pulls changes even when command exits non-zero", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-pull-nonzero-123"
		address := "192.168.1.1:22"

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		sandboxPatch := "diff --git a/file.txt b/file.txt\nindex abc..def 100644\n--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+new\n"

		setup.mockSSH.MockExecuteCommandWithOutput = func(cmd string) (int, string, error) {
			if strings.Contains(cmd, "git diff refs/rwx-sync") {
				return 0, sandboxPatch, nil
			}
			return 0, "", nil
		}

		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			if cmd == "make test" {
				return 1, nil // Command fails
			}
			return 0, nil
		}

		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return nil, nil, nil
		}

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"make", "test"},
			RunID:      runID,
			Json:       true,
			Sync:       true,
		})

		require.NoError(t, err)
		require.Equal(t, runID, result.RunID)
		require.Equal(t, 1, result.ExitCode)
		require.Contains(t, result.PulledFiles, "file.txt")
	})

	t.Run("pull uses stdout-only capture for git data commands", func(t *testing.T) {
		// Regression test for: "Fix sandbox pull erasing local uncommitted changes"
		//
		// Previously, pullChangesFromSandbox used ExecuteCommandWithCombinedOutput which
		// mixed stderr into stdout. When git commands produced warnings on stderr, the
		// patch data was corrupted. Since local files were already reset to HEAD before
		// applying the patch, corrupted patches caused local uncommitted changes to be erased.
		//
		// This test exercises the full pull path with untracked files on the sandbox,
		// verifying that git ls-files, git add -N, git diff refs/rwx-sync, and git reset HEAD
		// all go through ExecuteCommandWithOutput (stdout-only).
		setup := setupTest(t)

		runID := "run-pull-stdout-only"
		address := "192.168.1.1:22"
		var stdoutOnlyCommands []string

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		// The sandbox has a tracked change and an untracked file
		sandboxPatch := "diff --git a/tracked.txt b/tracked.txt\nindex abc..def 100644\n--- a/tracked.txt\n+++ b/tracked.txt\n@@ -1 +1 @@\n-old\n+new\ndiff --git a/untracked.txt b/untracked.txt\nnew file mode 100644\nindex 0000000..abc1234\n--- /dev/null\n+++ b/untracked.txt\n@@ -0,0 +1 @@\n+new file\n"

		setup.mockSSH.MockExecuteCommandWithOutput = func(cmd string) (int, string, error) {
			stdoutOnlyCommands = append(stdoutOnlyCommands, cmd)
			if strings.Contains(cmd, "git ls-files --others") {
				return 0, "untracked.txt\n", nil
			}
			if strings.Contains(cmd, "git diff refs/rwx-sync") {
				return 0, sandboxPatch, nil
			}
			return 0, "", nil
		}

		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			return 0, nil
		}

		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return nil, nil, nil
		}

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
			Sync:       true,
		})

		require.NoError(t, err)
		require.Equal(t, 0, result.ExitCode)
		require.Contains(t, result.PulledFiles, "tracked.txt")
		require.Contains(t, result.PulledFiles, "untracked.txt")

		// Verify that data-capturing git commands used stdout-only output
		// (these are called via ExecuteCommandWithOutput, not combined output)
		foundLsFiles := false
		foundAddN := false
		foundDiffRef := false
		foundReset := false
		for _, cmd := range stdoutOnlyCommands {
			if strings.Contains(cmd, "git ls-files --others") {
				foundLsFiles = true
			}
			if strings.Contains(cmd, "git add -N") {
				foundAddN = true
			}
			if strings.Contains(cmd, "git diff refs/rwx-sync") {
				foundDiffRef = true
			}
			if strings.Contains(cmd, "git reset HEAD") {
				foundReset = true
			}
		}
		require.True(t, foundLsFiles, "git ls-files should use stdout-only output")
		require.True(t, foundAddN, "git add -N should use stdout-only output")
		require.True(t, foundDiffRef, "git diff refs/rwx-sync should use stdout-only output")
		require.True(t, foundReset, "git reset HEAD should use stdout-only output")
	})

	t.Run("pull only includes sandbox exec-changed files", func(t *testing.T) {
		// With the sync snapshot commit, the sandbox diff only captures changes made
		// during exec (not local changes that were synced before exec). Local changes
		// are already present in the working tree and don't need to be pulled back.
		setup := setupTest(t)

		runID := "run-pull-local-changes"
		address := "192.168.1.1:22"

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		// Sandbox has changes to file-a.txt only (exec-only changes)
		sandboxPatch := "diff --git a/file-a.txt b/file-a.txt\nindex abc..def 100644\n--- a/file-a.txt\n+++ b/file-a.txt\n@@ -1 +1 @@\n-old\n+new\n"

		setup.mockSSH.MockExecuteCommandWithOutput = func(cmd string) (int, string, error) {
			if strings.Contains(cmd, "git diff refs/rwx-sync") {
				return 0, sandboxPatch, nil
			}
			return 0, "", nil
		}

		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			return 0, nil
		}

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
			Sync:       true,
		})

		require.NoError(t, err)
		require.Equal(t, 0, result.ExitCode)
		// Only sandbox exec-changed files should be in the pulled files list
		require.Contains(t, result.PulledFiles, "file-a.txt")
		require.Equal(t, 1, len(result.PulledFiles))
	})

	t.Run("treats pull errors as warnings", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-pull-err-123"
		address := "192.168.1.1:22"

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		setup.mockSSH.MockExecuteCommandWithOutput = func(cmd string) (int, string, error) {
			if strings.Contains(cmd, "git ls-files") {
				return 1, "fatal: not a git repository", nil // Pull fails
			}
			return 0, "", nil
		}

		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			return 0, nil
		}

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
			Sync:       true,
		})

		require.NoError(t, err)
		require.Equal(t, runID, result.RunID)
		require.Equal(t, 0, result.ExitCode)
		require.Contains(t, setup.mockStderr.String(), "Warning: failed to pull changes from sandbox")
	})

	t.Run("skips pull when Sync is false", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-pull-no-sync-123"
		address := "192.168.1.1:22"
		gitDiffCalled := false

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		// Sandbox has changes, but pull should never be attempted
		sandboxPatch := "diff --git a/file.txt b/file.txt\nindex abc..def 100644\n--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+new\n"

		setup.mockSSH.MockExecuteCommandWithOutput = func(cmd string) (int, string, error) {
			if strings.Contains(cmd, "git diff refs/rwx-sync") {
				gitDiffCalled = true
				return 0, sandboxPatch, nil
			}
			return 0, "", nil
		}

		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			return 0, nil
		}

		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return nil, nil, nil
		}

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
			Sync:       false,
		})

		require.NoError(t, err)
		require.Equal(t, runID, result.RunID)
		require.Equal(t, 0, result.ExitCode)
		require.Empty(t, result.PulledFiles, "pull should be skipped when Sync is false")
		require.False(t, gitDiffCalled, "git diff refs/rwx-sync should not be called when Sync is false")
	})
}

func TestService_ExecSandbox_PullPatchFailureRecovery(t *testing.T) {
	t.Run("retries with --reject on apply failure and reports .rej files", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-reject-123"
		address := "192.168.1.1:22"

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		sandboxPatch := "diff --git a/file.txt b/file.txt\nindex abc..def 100644\n--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+new\n"

		setup.mockSSH.MockExecuteCommandWithOutput = func(cmd string) (int, string, error) {
			if strings.Contains(cmd, "git diff refs/rwx-sync") {
				return 0, sandboxPatch, nil
			}
			return 0, "", nil
		}

		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			return 0, nil
		}

		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return nil, nil, nil
		}

		// First apply fails
		setup.mockGit.MockApplyPatch = func(patch []byte) *exec.Cmd {
			return exec.Command("false")
		}

		// --reject also fails (partial apply), and create a .rej file to simulate
		setup.mockGit.MockApplyPatchReject = func(patch []byte) *exec.Cmd {
			// Create a .rej file to simulate partial apply
			_ = os.WriteFile(filepath.Join(setup.tmp, "file.txt.rej"), []byte("rejected hunk"), 0644)
			return exec.Command("false")
		}

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
			Sync:       true,
		})

		require.NoError(t, err)
		require.Equal(t, runID, result.RunID)
		// Files should still be returned even though apply partially failed
		require.Contains(t, result.PulledFiles, "file.txt")
		// Warning should mention .rej files
		require.Contains(t, setup.mockStderr.String(), "file.txt (see file.txt.rej)")
		require.Contains(t, setup.mockStderr.String(), "Resolve the conflicts")
	})

	t.Run("cleans up saved patch when --reject succeeds fully", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-reject-ok-123"
		address := "192.168.1.1:22"

		// Create .rwx directory so the patch can be saved
		rwxDir := filepath.Join(setup.tmp, ".rwx", "sandboxes")
		require.NoError(t, os.MkdirAll(rwxDir, 0755))

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		sandboxPatch := "diff --git a/file.txt b/file.txt\nindex abc..def 100644\n--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+new\n"

		setup.mockSSH.MockExecuteCommandWithOutput = func(cmd string) (int, string, error) {
			if strings.Contains(cmd, "git diff refs/rwx-sync") {
				return 0, sandboxPatch, nil
			}
			return 0, "", nil
		}

		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			return 0, nil
		}

		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return nil, nil, nil
		}

		// First apply fails
		setup.mockGit.MockApplyPatch = func(patch []byte) *exec.Cmd {
			return exec.Command("false")
		}

		// --reject succeeds (all hunks applied on retry)
		setup.mockGit.MockApplyPatchReject = func(patch []byte) *exec.Cmd {
			return exec.Command("true")
		}

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
			Sync:       true,
		})

		require.NoError(t, err)
		require.Contains(t, result.PulledFiles, "file.txt")
		// Saved patch should be cleaned up since --reject succeeded
		_, statErr := os.Stat(filepath.Join(rwxDir, "patch-rejected.diff"))
		require.True(t, os.IsNotExist(statErr), "patch file should be cleaned up when --reject succeeds")
		// No warning should be printed
		require.NotContains(t, setup.mockStderr.String(), "conflicts")
	})

	t.Run("saves rejected patch to .rwx/sandboxes", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-save-patch-123"
		address := "192.168.1.1:22"

		// Create .rwx directory
		rwxDir := filepath.Join(setup.tmp, ".rwx", "sandboxes")
		require.NoError(t, os.MkdirAll(rwxDir, 0755))

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		sandboxPatch := "diff --git a/file.txt b/file.txt\nindex abc..def 100644\n--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+new\n"

		setup.mockSSH.MockExecuteCommandWithOutput = func(cmd string) (int, string, error) {
			if strings.Contains(cmd, "git diff refs/rwx-sync") {
				return 0, sandboxPatch, nil
			}
			return 0, "", nil
		}

		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			return 0, nil
		}

		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return nil, nil, nil
		}

		// Both apply attempts fail
		setup.mockGit.MockApplyPatch = func(patch []byte) *exec.Cmd {
			return exec.Command("false")
		}
		setup.mockGit.MockApplyPatchReject = func(patch []byte) *exec.Cmd {
			return exec.Command("false")
		}

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
			Sync:       true,
		})

		require.NoError(t, err)
		require.Equal(t, runID, result.RunID)

		// Verify the patch was saved
		savedPatch, readErr := os.ReadFile(filepath.Join(rwxDir, "patch-rejected.diff"))
		require.NoError(t, readErr)
		require.Equal(t, sandboxPatch, string(savedPatch))

		// Warning should mention the saved patch path
		require.Contains(t, setup.mockStderr.String(), "patch-rejected.diff")
	})

	t.Run("warns about .rej files during sync", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-rej-warn-123"
		address := "192.168.1.1:22"

		// Create a .rej file from a "previous" failed pull
		require.NoError(t, os.MkdirAll(filepath.Join(setup.tmp, "src"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(setup.tmp, "src", "main.go.rej"), []byte("old reject"), 0644))

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		setup.mockSSH.MockExecuteCommandWithOutput = func(cmd string) (int, string, error) {
			return 0, "", nil
		}

		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			return 0, nil
		}

		// No local changes (sync still runs the .rej check)
		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return nil, nil, nil
		}

		_, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
			Sync:       true,
		})

		require.NoError(t, err)
		require.Contains(t, setup.mockStderr.String(), "unresolved .rej file(s)")
		require.Contains(t, setup.mockStderr.String(), "src/main.go.rej")
		require.Contains(t, setup.mockStderr.String(), "Resolve and delete them when possible")
	})
}

func TestService_StartSandbox(t *testing.T) {
	t.Run("does not send a git patch", func(t *testing.T) {
		setup := setupTest(t)

		// Create .rwx directory and sandbox config file
		rwxDir := filepath.Join(setup.tmp, ".rwx")
		err := os.MkdirAll(rwxDir, 0o755)
		require.NoError(t, err)

		sandboxConfig := "tasks:\n  - key: sandbox\n    run: rwx-sandbox\n"
		err = os.WriteFile(filepath.Join(rwxDir, "sandbox.yml"), []byte(sandboxConfig), 0o644)
		require.NoError(t, err)

		// Mock git — set up a patch that would be sent if patching were enabled
		setup.mockGit.MockGetBranch = "main"
		setup.mockGit.MockGetCommit = "abc123"
		setup.mockGit.MockGetOriginUrl = "git@github.com:example/repo.git"
		setup.mockGit.MockGeneratePatchFile = git.PatchFile{
			Written:        true,
			UntrackedFiles: git.UntrackedFilesMetadata{Files: []string{"foo.txt"}, Count: 1},
		}

		// Mock API
		setup.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
			return api.DefaultBaseResult{
				Image:  "ubuntu:24.04",
				Config: "rwx/base 1.0.0",
				Arch:   "x86_64",
			}, nil
		}
		setup.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
			return &api.PackageVersionsResult{
				LatestMajor: make(map[string]string),
				LatestMinor: make(map[string]map[string]string),
			}, nil
		}

		var receivedPatch api.PatchMetadata
		setup.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			receivedPatch = cfg.Patch
			return &api.InitiateRunResult{
				RunID:  "run-123",
				RunURL: "https://cloud.rwx.com/mint/runs/run-123",
			}, nil
		}
		setup.mockAPI.MockCreateSandboxToken = func(cfg api.CreateSandboxTokenConfig) (*api.CreateSandboxTokenResult, error) {
			return &api.CreateSandboxTokenResult{Token: "test-token"}, nil
		}

		_, err = setup.service.StartSandbox(cli.StartSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Json:       true,
		})

		require.NoError(t, err)
		require.False(t, receivedPatch.Sent, "sandbox start should not send a git patch")
	})

	t.Run("passes init params to InitiateRun", func(t *testing.T) {
		setup := setupTest(t)

		// Create .rwx directory and sandbox config file
		rwxDir := filepath.Join(setup.tmp, ".rwx")
		err := os.MkdirAll(rwxDir, 0o755)
		require.NoError(t, err)

		sandboxConfig := "tasks:\n  - key: sandbox\n    run: rwx-sandbox\n"
		err = os.WriteFile(filepath.Join(rwxDir, "sandbox.yml"), []byte(sandboxConfig), 0o644)
		require.NoError(t, err)

		// Mock git
		setup.mockGit.MockGetBranch = "main"
		setup.mockGit.MockGetCommit = "abc123"
		setup.mockGit.MockGetOriginUrl = "git@github.com:example/repo.git"

		// Mock API
		setup.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
			return api.DefaultBaseResult{
				Image:  "ubuntu:24.04",
				Config: "rwx/base 1.0.0",
				Arch:   "x86_64",
			}, nil
		}
		setup.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
			return &api.PackageVersionsResult{
				LatestMajor: make(map[string]string),
				LatestMinor: make(map[string]map[string]string),
			}, nil
		}

		var receivedInitParams []api.InitializationParameter
		setup.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			receivedInitParams = cfg.InitializationParameters
			return &api.InitiateRunResult{
				RunID:  "run-init-123",
				RunURL: "https://cloud.rwx.com/mint/runs/run-init-123",
			}, nil
		}
		setup.mockAPI.MockCreateSandboxToken = func(cfg api.CreateSandboxTokenConfig) (*api.CreateSandboxTokenResult, error) {
			return &api.CreateSandboxTokenResult{Token: "test-token"}, nil
		}

		result, err := setup.service.StartSandbox(cli.StartSandboxConfig{
			ConfigFile:     setup.absConfig(".rwx/sandbox.yml"),
			Json:           true,
			InitParameters: map[string]string{"foo": "bar"},
		})

		require.NoError(t, err)
		require.Equal(t, "run-init-123", result.RunID)
		require.Equal(t, "https://cloud.rwx.com/mint/runs/run-init-123", result.RunURL)
		require.Len(t, receivedInitParams, 1)
		require.Equal(t, "foo", receivedInitParams[0].Key)
		require.Equal(t, "bar", receivedInitParams[0].Value)
	})
}

func TestService_StartSandbox_StorageLock(t *testing.T) {
	t.Run("uses caller-provided lock and persists session", func(t *testing.T) {
		setup := setupTest(t)

		rwxDir := filepath.Join(setup.tmp, ".rwx")
		require.NoError(t, os.MkdirAll(rwxDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(rwxDir, "sandbox.yml"), []byte("tasks:\n  - key: sandbox\n    run: rwx-sandbox\n"), 0o644))

		setup.mockGit.MockGetBranch = "main"
		setup.mockGit.MockGetCommit = "abc123"
		setup.mockGit.MockGetOriginUrl = "git@github.com:example/repo.git"

		setup.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
			return api.DefaultBaseResult{Image: "ubuntu:24.04", Config: "rwx/base 1.0.0", Arch: "x86_64"}, nil
		}
		setup.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
			return &api.PackageVersionsResult{
				LatestMajor: make(map[string]string),
				LatestMinor: make(map[string]map[string]string),
			}, nil
		}

		setup.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			return &api.InitiateRunResult{
				RunID:  "run-lock-test",
				RunURL: "https://cloud.rwx.com/mint/runs/run-lock-test",
			}, nil
		}
		setup.mockAPI.MockCreateSandboxToken = func(cfg api.CreateSandboxTokenConfig) (*api.CreateSandboxTokenResult, error) {
			return &api.CreateSandboxTokenResult{Token: "test-token"}, nil
		}

		// Pre-acquire the lock and pass it to StartSandbox
		lock, err := cli.LockSandboxStorage()
		require.NoError(t, err)

		result, err := setup.service.StartSandbox(cli.StartSandboxConfigWithLock(cli.StartSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Json:       true,
		}, lock))

		require.NoError(t, err)
		require.Equal(t, "run-lock-test", result.RunID)

		// Verify session was persisted
		storage, err := cli.LoadSandboxStorage()
		require.NoError(t, err)
		require.NotEmpty(t, storage.Sandboxes, "expected at least one session in storage")
		// The temp dir is not a git repo, so the branch resolves to "detached"
		session, found := storage.GetSession("detached", setup.absConfig(".rwx/sandbox.yml"))
		require.True(t, found)
		require.Equal(t, "run-lock-test", session.RunID)

		// Verify lock was released (we can acquire it again)
		newLock, err := cli.TryLockSandboxStorage()
		require.NoError(t, err)
		cli.UnlockSandboxStorage(newLock)
	})

	t.Run("releases caller-provided lock on InitiateRun failure", func(t *testing.T) {
		setup := setupTest(t)

		rwxDir := filepath.Join(setup.tmp, ".rwx")
		require.NoError(t, os.MkdirAll(rwxDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(rwxDir, "sandbox.yml"), []byte("tasks:\n  - key: sandbox\n    run: rwx-sandbox\n"), 0o644))

		setup.mockGit.MockGetBranch = "main"
		setup.mockGit.MockGetCommit = "abc123"
		setup.mockGit.MockGetOriginUrl = "git@github.com:example/repo.git"

		setup.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
			return api.DefaultBaseResult{Image: "ubuntu:24.04", Config: "rwx/base 1.0.0", Arch: "x86_64"}, nil
		}
		setup.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
			return &api.PackageVersionsResult{
				LatestMajor: make(map[string]string),
				LatestMinor: make(map[string]map[string]string),
			}, nil
		}

		setup.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			return nil, fmt.Errorf("API error")
		}

		lock, err := cli.LockSandboxStorage()
		require.NoError(t, err)

		_, err = setup.service.StartSandbox(cli.StartSandboxConfigWithLock(cli.StartSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Json:       true,
		}, lock))

		require.Error(t, err)

		// Verify lock was released despite the error
		newLock, err := cli.TryLockSandboxStorage()
		require.NoError(t, err)
		cli.UnlockSandboxStorage(newLock)
	})
}

func TestService_ExecSandbox_ConcurrentAutoCreate(t *testing.T) {
	t.Run("concurrent exec calls create only one sandbox", func(t *testing.T) {
		setup := setupTest(t)

		rwxDir := filepath.Join(setup.tmp, ".rwx")
		require.NoError(t, os.MkdirAll(rwxDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(rwxDir, "sandbox.yml"), []byte("tasks:\n  - key: sandbox\n    run: rwx-sandbox\n"), 0o644))

		address := "192.168.1.1:22"

		setup.mockGit.MockGetBranch = "main"
		setup.mockGit.MockGetCommit = "abc123"
		setup.mockGit.MockGetOriginUrl = "git@github.com:example/repo.git"

		setup.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
			return api.DefaultBaseResult{Image: "ubuntu:24.04", Config: "rwx/base 1.0.0", Arch: "x86_64"}, nil
		}
		setup.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
			return &api.PackageVersionsResult{
				LatestMajor: make(map[string]string),
				LatestMinor: make(map[string]map[string]string),
			}, nil
		}

		var initiateRunCount int32
		setup.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			newCount := atomic.AddInt32(&initiateRunCount, 1)
			runID := fmt.Sprintf("run-%d", newCount)
			return &api.InitiateRunResult{
				RunID:  runID,
				RunURL: fmt.Sprintf("https://cloud.rwx.com/mint/runs/%s", runID),
			}, nil
		}
		setup.mockAPI.MockCreateSandboxToken = func(cfg api.CreateSandboxTokenConfig) (*api.CreateSandboxTokenResult, error) {
			return &api.CreateSandboxTokenResult{Token: "test-token"}, nil
		}
		setup.mockAPI.MockListSandboxRuns = func() (*api.ListSandboxRunsResult, error) {
			return &api.ListSandboxRunsResult{Runs: []api.SandboxRunSummary{}}, nil
		}
		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}
		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			return 0, nil
		}
		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return nil, nil, nil
		}

		// Create a second service that shares the same mock API and filesystem
		// but has its own stdout/stderr buffers.
		stdout2 := &strings.Builder{}
		stderr2 := &strings.Builder{}
		service2, err := cli.NewService(cli.Config{
			APIClient:   setup.mockAPI,
			SSHClient:   setup.mockSSH,
			GitClient:   setup.mockGit,
			DockerCLI:   setup.mockDocker,
			Stdin:       &bytes.Buffer{},
			Stdout:      stdout2,
			StdoutIsTTY: false,
			Stderr:      stderr2,
			StderrIsTTY: false,
		})
		require.NoError(t, err)

		var wg sync.WaitGroup
		wg.Add(2)

		var result1 *cli.ExecSandboxResult
		var err1 error
		go func() {
			defer wg.Done()
			result1, err1 = setup.service.ExecSandbox(cli.ExecSandboxConfig{
				Command: []string{"echo", "hello"},
				Json:    true,
			})
		}()

		var result2 *cli.ExecSandboxResult
		var err2 error
		go func() {
			defer wg.Done()
			result2, err2 = service2.ExecSandbox(cli.ExecSandboxConfig{
				Command: []string{"echo", "world"},
				Json:    true,
			})
		}()

		wg.Wait()

		require.NoError(t, err1)
		require.NoError(t, err2)

		// Both should use the same run — only one InitiateRun call should have happened.
		finalCount := atomic.LoadInt32(&initiateRunCount)
		require.Equal(t, int32(1), finalCount, "only one sandbox should have been created, but got %d", finalCount)
		require.Equal(t, result1.RunID, result2.RunID, "both exec calls should use the same sandbox")
	})
}

func TestService_ExecSandbox_RecoverFromAPI(t *testing.T) {
	t.Run("reuses remote sandbox when no local session exists", func(t *testing.T) {
		setup := setupTest(t)

		// Set HOME so sandbox storage is writable in the test temp dir
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", setup.tmp)
		t.Cleanup(func() { os.Setenv("HOME", originalHome) })

		address := "192.168.1.1:22"
		// GetCurrentGitBranch uses a real git client, so in a non-repo temp dir it returns "detached"
		branch := "detached"
		configFile := setup.absConfig(".rwx/sandbox.yml")

		// Encode cli_state matching branch+configFile
		encodedState := cli.EncodeCliState(branch, configFile)

		// No local session — ListSandboxRuns returns a matching run
		setup.mockAPI.MockListSandboxRuns = func() (*api.ListSandboxRunsResult, error) {
			return &api.ListSandboxRunsResult{
				Runs: []api.SandboxRunSummary{
					{
						ID:       "run-recovered",
						RunURL:   "https://cloud.rwx.com/runs/run-recovered",
						CliState: encodedState,
					},
				},
			}, nil
		}

		setup.mockAPI.MockCreateSandboxToken = func(cfg api.CreateSandboxTokenConfig) (*api.CreateSandboxTokenResult, error) {
			require.Equal(t, "run-recovered", cfg.RunID)
			return &api.CreateSandboxTokenResult{Token: "recovered-token"}, nil
		}

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			require.Equal(t, "run-recovered", id)
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}
		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			return 0, nil
		}
		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return nil, nil, nil
		}

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: configFile,
			Command:    []string{"echo", "hello"},
			Json:       true,
		})

		require.NoError(t, err)
		require.Equal(t, "run-recovered", result.RunID)
		require.Equal(t, "https://cloud.rwx.com/runs/run-recovered", result.RunURL)

		// Verify the session was stored locally
		storage, err := cli.LoadSandboxStorage()
		require.NoError(t, err)
		session, found := storage.GetSession(branch, configFile)
		require.True(t, found)
		require.Equal(t, "run-recovered", session.RunID)
		require.Equal(t, "recovered-token", session.ScopedToken)
	})

	t.Run("falls through to auto-create when no remote match", func(t *testing.T) {
		setup := setupTest(t)

		// Set HOME so sandbox storage is writable in the test temp dir
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", setup.tmp)
		t.Cleanup(func() { os.Setenv("HOME", originalHome) })

		// Create .rwx directory and sandbox config file
		rwxDir := filepath.Join(setup.tmp, ".rwx")
		require.NoError(t, os.MkdirAll(rwxDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(rwxDir, "sandbox.yml"), []byte("tasks:\n  - key: sandbox\n    run: rwx-sandbox\n"), 0o644))

		address := "192.168.1.1:22"

		// ListSandboxRuns returns no matching runs
		setup.mockAPI.MockListSandboxRuns = func() (*api.ListSandboxRunsResult, error) {
			return &api.ListSandboxRunsResult{Runs: []api.SandboxRunSummary{}}, nil
		}

		// Mock the full auto-create path
		setup.mockGit.MockGetBranch = "main"
		setup.mockGit.MockGetCommit = "abc123"
		setup.mockGit.MockGetOriginUrl = "git@github.com:example/repo.git"

		setup.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
			return api.DefaultBaseResult{Image: "ubuntu:24.04", Config: "rwx/base 1.0.0", Arch: "x86_64"}, nil
		}
		setup.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
			return &api.PackageVersionsResult{
				LatestMajor: make(map[string]string),
				LatestMinor: make(map[string]map[string]string),
			}, nil
		}

		var initiatedRun bool
		setup.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			initiatedRun = true
			return &api.InitiateRunResult{
				RunID:  "run-new",
				RunURL: "https://cloud.rwx.com/mint/runs/run-new",
			}, nil
		}
		setup.mockAPI.MockCreateSandboxToken = func(cfg api.CreateSandboxTokenConfig) (*api.CreateSandboxTokenResult, error) {
			return &api.CreateSandboxTokenResult{Token: "new-token"}, nil
		}
		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}
		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			return 0, nil
		}
		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return nil, nil, nil
		}

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			Command: []string{"echo", "hello"},
			Json:    true,
		})

		require.NoError(t, err)
		require.Equal(t, "run-new", result.RunID)
		require.True(t, initiatedRun, "should have initiated a new run")
	})
}

func TestService_ExecSandbox_InitParams(t *testing.T) {
	t.Run("lazy-create passes init params through to InitiateRun", func(t *testing.T) {
		setup := setupTest(t)

		// Create .rwx directory and sandbox config file
		rwxDir := filepath.Join(setup.tmp, ".rwx")
		err := os.MkdirAll(rwxDir, 0o755)
		require.NoError(t, err)

		sandboxConfig := "tasks:\n  - key: sandbox\n    run: rwx-sandbox\n"
		err = os.WriteFile(filepath.Join(rwxDir, "sandbox.yml"), []byte(sandboxConfig), 0o644)
		require.NoError(t, err)

		// Mock git
		setup.mockGit.MockGetBranch = "main"
		setup.mockGit.MockGetCommit = "abc123"
		setup.mockGit.MockGetOriginUrl = "git@github.com:example/repo.git"

		// Mock API
		setup.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
			return api.DefaultBaseResult{
				Image:  "ubuntu:24.04",
				Config: "rwx/base 1.0.0",
				Arch:   "x86_64",
			}, nil
		}
		setup.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
			return &api.PackageVersionsResult{
				LatestMajor: make(map[string]string),
				LatestMinor: make(map[string]map[string]string),
			}, nil
		}

		var receivedInitParams []api.InitializationParameter
		setup.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			receivedInitParams = cfg.InitializationParameters
			return &api.InitiateRunResult{
				RunID:  "run-lazy-123",
				RunURL: "https://cloud.rwx.com/mint/runs/run-lazy-123",
			}, nil
		}
		setup.mockAPI.MockCreateSandboxToken = func(cfg api.CreateSandboxTokenConfig) (*api.CreateSandboxTokenResult, error) {
			return &api.CreateSandboxTokenResult{Token: "test-token"}, nil
		}

		address := "192.168.1.1:22"
		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}
		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			return 0, nil
		}
		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return nil, nil, nil
		}

		// No RunID provided - forces lazy-create path
		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			Command:        []string{"echo", "hello"},
			Json:           true,
			InitParameters: map[string]string{"foo": "bar"},
		})

		require.NoError(t, err)
		require.Equal(t, "run-lazy-123", result.RunID)
		require.Len(t, receivedInitParams, 1)
		require.Equal(t, "foo", receivedInitParams[0].Key)
		require.Equal(t, "bar", receivedInitParams[0].Value)
	})
}

func TestService_StopSandbox(t *testing.T) {
	t.Run("returns error when no sandbox exists for current directory", func(t *testing.T) {
		setup := setupTest(t)

		_, err := setup.service.StopSandbox(cli.StopSandboxConfig{
			Json: true,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "No sandbox found")
	})

	t.Run("returns error when sandbox ID not found in storage", func(t *testing.T) {
		setup := setupTest(t)

		_, err := setup.service.StopSandbox(cli.StopSandboxConfig{
			RunID: "nonexistent-run",
			Json:  true,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "not found in local storage")
	})

	t.Run("calls CancelRun for active but not yet sandboxable run", func(t *testing.T) {
		setup := setupTest(t)
		seedSandboxStorage(t, setup.tmp, "run-initializing", "scoped-token-123")

		cancelCalled := false
		setup.mockAPI.MockGetSandboxConnectionInfo = func(runID, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable: false,
				Polling:     api.PollingResult{Completed: false},
			}, nil
		}
		setup.mockAPI.MockCancelRun = func(runID, scopedToken string) error {
			require.Equal(t, "run-initializing", runID)
			require.Equal(t, "scoped-token-123", scopedToken)
			cancelCalled = true
			return nil
		}

		result, err := setup.service.StopSandbox(cli.StopSandboxConfig{
			RunID: "run-initializing",
			Json:  true,
		})

		require.NoError(t, err)
		require.True(t, cancelCalled, "CancelRun should have been called")
		require.Len(t, result.Stopped, 1)
		require.True(t, result.Stopped[0].WasRunning)

		stopEvent := findEvent(setup.drainEvents(), "sandbox.stop")
		require.NotNil(t, stopEvent)
		require.Equal(t, "api", stopEvent.Props["cancel_method"])
	})

	t.Run("logs warning when CancelRun fails but still succeeds", func(t *testing.T) {
		setup := setupTest(t)
		seedSandboxStorage(t, setup.tmp, "run-cancel-fail", "token-456")

		setup.mockAPI.MockGetSandboxConnectionInfo = func(runID, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable: false,
				Polling:     api.PollingResult{Completed: false},
			}, nil
		}
		setup.mockAPI.MockCancelRun = func(runID, scopedToken string) error {
			return errors.New("server error")
		}

		result, err := setup.service.StopSandbox(cli.StopSandboxConfig{
			RunID: "run-cancel-fail",
			Json:  true,
		})

		require.NoError(t, err)
		require.Len(t, result.Stopped, 1)
		require.True(t, result.Stopped[0].WasRunning)
		require.Contains(t, setup.mockStderr.String(), "Warning: failed to cancel run")
	})

	t.Run("does not call CancelRun for sandboxable run", func(t *testing.T) {
		setup := setupTest(t)
		seedSandboxStorage(t, setup.tmp, "run-sandboxable", "token-789")

		setup.mockAPI.MockGetSandboxConnectionInfo = func(runID, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        "192.168.1.1:22",
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}
		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}
		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			return 0, nil
		}
		setup.mockAPI.MockCancelRun = func(runID, scopedToken string) error {
			t.Fatal("CancelRun should not be called for sandboxable runs")
			return nil
		}

		result, err := setup.service.StopSandbox(cli.StopSandboxConfig{
			RunID: "run-sandboxable",
			Json:  true,
		})

		require.NoError(t, err)
		require.Len(t, result.Stopped, 1)
		require.True(t, result.Stopped[0].WasRunning)
	})

	t.Run("calls CancelRun when SSH connection fails for sandboxable run", func(t *testing.T) {
		setup := setupTest(t)
		seedSandboxStorage(t, setup.tmp, "run-ssh-fail", "token-ssh")

		setup.mockAPI.MockGetSandboxConnectionInfo = func(runID, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        "192.168.1.1:22",
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}
		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return errors.New("connection timed out")
		}
		cancelCalled := false
		setup.mockAPI.MockCancelRun = func(runID, scopedToken string) error {
			require.Equal(t, "run-ssh-fail", runID)
			require.Equal(t, "token-ssh", scopedToken)
			cancelCalled = true
			return nil
		}

		result, err := setup.service.StopSandbox(cli.StopSandboxConfig{
			RunID: "run-ssh-fail",
			Json:  true,
		})

		require.NoError(t, err)
		require.True(t, cancelCalled, "CancelRun should have been called when SSH fails")
		require.Len(t, result.Stopped, 1)
		require.True(t, result.Stopped[0].WasRunning)

		stopEvent := findEvent(setup.drainEvents(), "sandbox.stop")
		require.NotNil(t, stopEvent)
		require.Equal(t, "api", stopEvent.Props["cancel_method"])
	})

	t.Run("does not call CancelRun for completed run", func(t *testing.T) {
		setup := setupTest(t)
		seedSandboxStorage(t, setup.tmp, "run-completed", "token-done")

		setup.mockAPI.MockGetSandboxConnectionInfo = func(runID, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable: false,
				Polling:     api.PollingResult{Completed: true},
			}, nil
		}
		setup.mockAPI.MockCancelRun = func(runID, scopedToken string) error {
			t.Fatal("CancelRun should not be called for completed runs")
			return nil
		}

		result, err := setup.service.StopSandbox(cli.StopSandboxConfig{
			RunID: "run-completed",
			Json:  true,
		})

		require.NoError(t, err)
		require.Len(t, result.Stopped, 1)
		require.False(t, result.Stopped[0].WasRunning)
	})
}

func TestService_ResetSandbox(t *testing.T) {
	// setupResetMocks configures the mocks needed for StartSandbox to succeed after the old sandbox is stopped.
	setupResetMocks := func(setup *testSetup) {
		configPath := filepath.Join(setup.tmp, ".rwx", "sandbox.yml")
		_ = os.WriteFile(configPath, []byte("tasks:\n  - key: test\n"), 0o644)

		setup.mockGit.MockGetBranch = "main"
		setup.mockGit.MockGetCommit = "abc123"
		setup.mockGit.MockGetOriginUrl = "https://github.com/test/repo"
		setup.mockGit.MockGeneratePatchFile = git.PatchFile{}

		setup.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			return &api.InitiateRunResult{
				RunID:  "run-new",
				RunURL: "https://cloud.rwx.com/runs/run-new",
			}, nil
		}
		setup.mockAPI.MockCreateSandboxToken = func(cfg api.CreateSandboxTokenConfig) (*api.CreateSandboxTokenResult, error) {
			return &api.CreateSandboxTokenResult{Token: "token"}, nil
		}
		setup.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
			return api.DefaultBaseResult{}, nil
		}
		setup.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
			return &api.PackageVersionsResult{}, nil
		}
	}

	// seedResetStorage initializes a git repo on "main" and writes a sandbox session
	// keyed by branch+configFile so ResetSandbox can find it via GetCurrentGitBranch.
	seedResetStorage := func(t *testing.T, setup *testSetup, runID, scopedToken string) {
		t.Helper()

		// GetCurrentGitBranch uses a real git client, so the temp dir must be a repo.
		cmd := exec.Command("git", "init", "-b", "main")
		cmd.Dir = setup.tmp
		require.NoError(t, cmd.Run())
		cmd = exec.Command("git", "commit", "--allow-empty", "-m", "init")
		cmd.Dir = setup.tmp
		require.NoError(t, cmd.Run())

		storageDir := filepath.Join(setup.tmp, ".rwx", "sandboxes")
		require.NoError(t, os.MkdirAll(storageDir, 0o755))

		configFile := filepath.Join(setup.tmp, ".rwx", "sandbox.yml")
		key := cli.SessionKey("main", configFile)
		storage := cli.SandboxStorage{
			Version: 1,
			Sandboxes: map[string]cli.SandboxSession{
				key: {
					RunID:       runID,
					ConfigFile:  configFile,
					ScopedToken: scopedToken,
				},
			},
		}
		data, err := json.Marshal(storage)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(storageDir, "sandboxes.json"), data, 0o644))
	}

	t.Run("calls CancelRun for active but not yet sandboxable run", func(t *testing.T) {
		setup := setupTest(t)
		setupResetMocks(setup)
		seedResetStorage(t, setup, "run-initializing", "scoped-token-123")

		cancelCalled := false
		setup.mockAPI.MockGetSandboxConnectionInfo = func(runID, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable: false,
				Polling:     api.PollingResult{Completed: false},
			}, nil
		}
		setup.mockAPI.MockCancelRun = func(runID, scopedToken string) error {
			require.Equal(t, "run-initializing", runID)
			require.Equal(t, "scoped-token-123", scopedToken)
			cancelCalled = true
			return nil
		}

		result, err := setup.service.ResetSandbox(cli.ResetSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Json:       true,
		})

		require.NoError(t, err)
		require.True(t, cancelCalled, "CancelRun should have been called")
		require.Equal(t, "run-initializing", result.OldRunID)
		require.Equal(t, "run-new", result.NewRunID)

		resetEvent := findEvent(setup.drainEvents(), "sandbox.reset")
		require.NotNil(t, resetEvent)
		require.Equal(t, "api", resetEvent.Props["cancel_method"])
	})

	t.Run("calls CancelRun when SSH connection fails for sandboxable run", func(t *testing.T) {
		setup := setupTest(t)
		setupResetMocks(setup)
		seedResetStorage(t, setup, "run-ssh-fail", "token-ssh")

		setup.mockAPI.MockGetSandboxConnectionInfo = func(runID, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        "192.168.1.1:22",
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}
		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return errors.New("connection timed out")
		}
		cancelCalled := false
		setup.mockAPI.MockCancelRun = func(runID, scopedToken string) error {
			require.Equal(t, "run-ssh-fail", runID)
			require.Equal(t, "token-ssh", scopedToken)
			cancelCalled = true
			return nil
		}

		result, err := setup.service.ResetSandbox(cli.ResetSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Json:       true,
		})

		require.NoError(t, err)
		require.True(t, cancelCalled, "CancelRun should have been called when SSH fails")
		require.Equal(t, "run-ssh-fail", result.OldRunID)
		require.Equal(t, "run-new", result.NewRunID)

		resetEvent := findEvent(setup.drainEvents(), "sandbox.reset")
		require.NotNil(t, resetEvent)
		require.Equal(t, "api", resetEvent.Props["cancel_method"])
	})

	t.Run("does not call CancelRun for completed run", func(t *testing.T) {
		setup := setupTest(t)
		setupResetMocks(setup)
		seedResetStorage(t, setup, "run-completed", "token-done")

		setup.mockAPI.MockGetSandboxConnectionInfo = func(runID, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable: false,
				Polling:     api.PollingResult{Completed: true},
			}, nil
		}
		setup.mockAPI.MockCancelRun = func(runID, scopedToken string) error {
			t.Fatal("CancelRun should not be called for completed runs")
			return nil
		}

		result, err := setup.service.ResetSandbox(cli.ResetSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Json:       true,
		})

		require.NoError(t, err)
		require.Equal(t, "run-completed", result.OldRunID)
		require.Equal(t, "run-new", result.NewRunID)

		resetEvent := findEvent(setup.drainEvents(), "sandbox.reset")
		require.NotNil(t, resetEvent)
		require.Equal(t, "", resetEvent.Props["cancel_method"])
	})
}

// seedSandboxStorage writes a sandbox session into storage within the test's temp HOME directory.
func seedSandboxStorage(t *testing.T, tmpHome, runID, scopedToken string) {
	t.Helper()

	storageDir := filepath.Join(tmpHome, ".rwx", "sandboxes")
	require.NoError(t, os.MkdirAll(storageDir, 0o755))

	configFile := filepath.Join(tmpHome, ".rwx", "sandbox.yml")
	storage := cli.SandboxStorage{
		Version: 1,
		Sandboxes: map[string]cli.SandboxSession{
			"test-key": {
				RunID:       runID,
				ConfigFile:  configFile,
				ScopedToken: scopedToken,
			},
		},
	}

	data, err := json.Marshal(storage)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(storageDir, "sandboxes.json"), data, 0o644))
}

func TestService_ExecSandbox_RunURL(t *testing.T) {
	t.Run("returns empty RunURL when no server-provided URL is stored", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-no-url"
		address := "192.168.1.1:22"

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			return 0, nil
		}

		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return nil, nil, nil
		}

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
		})

		require.NoError(t, err)
		require.Equal(t, "", result.RunURL)
	})
}

func TestService_StartSandbox_RunURL(t *testing.T) {
	t.Run("reattach via --id returns empty RunURL", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-reattach-no-url"

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable: true,
				Polling:     api.PollingResult{Completed: false},
			}, nil
		}

		setup.mockAPI.MockCreateSandboxToken = func(cfg api.CreateSandboxTokenConfig) (*api.CreateSandboxTokenResult, error) {
			return &api.CreateSandboxTokenResult{Token: "test-token"}, nil
		}

		result, err := setup.service.StartSandbox(cli.StartSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			RunID:      runID,
			Json:       true,
		})

		require.NoError(t, err)
		require.Equal(t, "", result.RunURL)
	})

	t.Run("normal start uses server-provided RunURL", func(t *testing.T) {
		setup := setupTest(t)

		// Create .rwx directory and sandbox config file
		rwxDir := filepath.Join(setup.tmp, ".rwx")
		err := os.MkdirAll(rwxDir, 0o755)
		require.NoError(t, err)

		sandboxConfig := "tasks:\n  - key: sandbox\n    run: rwx-sandbox\n"
		err = os.WriteFile(filepath.Join(rwxDir, "sandbox.yml"), []byte(sandboxConfig), 0o644)
		require.NoError(t, err)

		setup.mockGit.MockGetBranch = "main"
		setup.mockGit.MockGetCommit = "abc123"
		setup.mockGit.MockGetOriginUrl = "git@github.com:example/repo.git"

		setup.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
			return api.DefaultBaseResult{
				Image:  "ubuntu:24.04",
				Config: "rwx/base 1.0.0",
				Arch:   "x86_64",
			}, nil
		}
		setup.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
			return &api.PackageVersionsResult{
				LatestMajor: make(map[string]string),
				LatestMinor: make(map[string]map[string]string),
			}, nil
		}

		setup.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			return &api.InitiateRunResult{
				RunID:  "run-server-url",
				RunURL: "https://cloud.rwx.com/mint/my-org/runs/run-server-url",
			}, nil
		}
		setup.mockAPI.MockCreateSandboxToken = func(cfg api.CreateSandboxTokenConfig) (*api.CreateSandboxTokenResult, error) {
			return &api.CreateSandboxTokenResult{Token: "test-token"}, nil
		}

		result, err := setup.service.StartSandbox(cli.StartSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Json:       true,
		})

		require.NoError(t, err)
		// Should use the server-provided URL (which includes org slug), not a constructed one
		require.Equal(t, "https://cloud.rwx.com/mint/my-org/runs/run-server-url", result.RunURL)
	})
}

func TestService_ExecSandbox_Lock(t *testing.T) {
	t.Run("lock_requested precedes sync and exec, lock_released is last", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-lock-order"
		address := "192.168.1.1:22"

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return nil, nil, nil
		}

		setup.mockSSH.MockExecuteCommandWithOutput = func(cmd string) (int, string, error) {
			return 0, "", nil
		}

		var commandOrder []string
		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			commandOrder = append(commandOrder, cmd)
			return 0, nil
		}

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
			Sync:       true,
		})

		require.NoError(t, err)
		require.Equal(t, 0, result.ExitCode)
		require.Greater(t, len(commandOrder), 2)
		require.Equal(t, "__rwx_sandbox_lock_requested__", commandOrder[0])
		require.Equal(t, "__rwx_sandbox_lock_released__", commandOrder[len(commandOrder)-1])
	})

	t.Run("lock_released is sent when sync fails", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-lock-sync-fail"
		address := "192.168.1.1:22"

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return []byte("invalid patch"), nil, nil
		}

		var commandOrder []string
		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			commandOrder = append(commandOrder, cmd)
			return 0, nil
		}

		setup.mockSSH.MockExecuteCommandWithOutput = func(cmd string) (int, string, error) {
			return 0, "", nil
		}

		setup.mockSSH.MockExecuteCommandWithStdinAndCombinedOutput = func(command string, stdin io.Reader) (int, string, error) {
			return 1, "error: patch failed", nil
		}

		_, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
			Sync:       true,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "git apply failed")
		require.Equal(t, "__rwx_sandbox_lock_requested__", commandOrder[0])
		require.Equal(t, "__rwx_sandbox_lock_released__", commandOrder[len(commandOrder)-1])
	})

	t.Run("lock_released is not sent when lock_requested fails", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-lock-fail"
		address := "192.168.1.1:22"

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		var commandOrder []string
		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			commandOrder = append(commandOrder, cmd)
			if cmd == "__rwx_sandbox_lock_requested__" {
				return -1, fmt.Errorf("SSH connection died")
			}
			return 0, nil
		}

		_, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to acquire sandbox lock")
		for _, cmd := range commandOrder {
			require.NotEqual(t, "__rwx_sandbox_lock_released__", cmd)
		}
	})

	t.Run("pre-exec cleanup runs between lock and sync", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-lock-clean"
		address := "192.168.1.1:22"

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return nil, nil, nil
		}

		setup.mockSSH.MockExecuteCommandWithOutput = func(cmd string) (int, string, error) {
			return 0, "", nil
		}

		var commandOrder []string
		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			commandOrder = append(commandOrder, cmd)
			return 0, nil
		}

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
			Sync:       true,
		})

		require.NoError(t, err)
		require.Equal(t, 0, result.ExitCode)

		// Verify cleanup commands appear after lock but before the user command
		cleanCmd := "/usr/bin/git checkout . >/dev/null 2>&1; /usr/bin/git clean -fd >/dev/null 2>&1"
		lockIdx := -1
		cleanStartIdx := -1
		cleanIdx := -1
		cleanEndIdx := -1
		execIdx := -1
		for i, cmd := range commandOrder {
			switch cmd {
			case "__rwx_sandbox_lock_requested__":
				lockIdx = i
			case "__rwx_sandbox_sync_start__":
				if cleanStartIdx == -1 {
					cleanStartIdx = i
				}
			case cleanCmd:
				cleanIdx = i
			case "__rwx_sandbox_sync_end__":
				if cleanEndIdx == -1 {
					cleanEndIdx = i
				}
			case "echo hello":
				execIdx = i
			}
		}

		require.Greater(t, cleanStartIdx, lockIdx, "cleanup sync_start should follow lock")
		require.Greater(t, cleanIdx, cleanStartIdx, "cleanup command should follow sync_start")
		require.Greater(t, cleanEndIdx, cleanIdx, "cleanup sync_end should follow cleanup command")
		require.Greater(t, execIdx, cleanEndIdx, "exec should follow cleanup")
	})

	t.Run("pre-exec cleanup is skipped when Sync is false", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-no-clean"
		address := "192.168.1.1:22"

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return nil, nil, nil
		}

		cleanCmd := "/usr/bin/git checkout . >/dev/null 2>&1; /usr/bin/git clean -fd >/dev/null 2>&1"
		cleanRan := false

		var commandOrder []string
		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			commandOrder = append(commandOrder, cmd)
			if cmd == cleanCmd {
				cleanRan = true
			}
			return 0, nil
		}

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
			Sync:       false,
		})

		require.NoError(t, err)
		require.Equal(t, 0, result.ExitCode)
		require.False(t, cleanRan, "cleanup should not run when Sync is false")
	})

	t.Run("lock_released is sent when user command exits non-zero", func(t *testing.T) {
		setup := setupTest(t)

		runID := "run-lock-nonzero"
		address := "192.168.1.1:22"

		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        address,
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}

		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
			return nil
		}

		setup.mockGit.MockGeneratePatch = func(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error) {
			return nil, nil, nil
		}

		setup.mockSSH.MockExecuteCommandWithOutput = func(cmd string) (int, string, error) {
			return 0, "", nil
		}

		var commandOrder []string
		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) {
			commandOrder = append(commandOrder, cmd)
			if cmd == "false" {
				return 1, nil
			}
			return 0, nil
		}

		result, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: setup.absConfig(".rwx/sandbox.yml"),
			Command:    []string{"false"},
			RunID:      runID,
			Json:       true,
		})

		require.NoError(t, err)
		require.Equal(t, 1, result.ExitCode)
		require.Equal(t, "__rwx_sandbox_lock_requested__", commandOrder[0])
		require.Equal(t, "__rwx_sandbox_lock_released__", commandOrder[len(commandOrder)-1])
	})
}

func TestService_ExecSandbox_DefinitionDrift(t *testing.T) {
	driftExecSetup := func(t *testing.T, setup *testSetup, runID string) {
		t.Helper()
		setup.mockAPI.MockGetSandboxConnectionInfo = func(id, token string) (api.SandboxConnectionInfo, error) {
			return api.SandboxConnectionInfo{
				Sandboxable:    true,
				Address:        "192.168.1.1:22",
				PrivateUserKey: sandboxPrivateTestKey,
				PublicHostKey:  sandboxPublicTestKey,
			}, nil
		}
		setup.mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error { return nil }
		setup.mockSSH.MockExecuteCommand = func(cmd string) (int, error) { return 0, nil }
	}

	t.Run("warns when config file has changed since sandbox was started", func(t *testing.T) {
		setup := setupTest(t)
		runID := "run-drift"
		configFile := setup.absConfig(".rwx/sandbox.yml")
		configPath := configFile

		require.NoError(t, os.WriteFile(configPath, []byte("tasks:\n  - key: original\n"), 0o644))
		originalHash := cli.HashConfigFile(configPath)

		// Seed storage with original hash
		seedSandboxStorageMulti(t, setup.tmp, map[string]cli.SandboxSession{
			"test-key": {
				RunID:      runID,
				ConfigFile: configFile,
				ConfigHash: originalHash,
			},
		})

		// Modify the config file
		require.NoError(t, os.WriteFile(configPath, []byte("tasks:\n  - key: modified\n"), 0o644))

		driftExecSetup(t, setup, runID)

		_, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: configFile,
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
		})

		require.NoError(t, err)
		require.Contains(t, setup.mockStderr.String(), "has changed since this sandbox was started")
		require.Contains(t, setup.mockStderr.String(), "rwx sandbox reset")
	})

	t.Run("does not warn when config file has not changed", func(t *testing.T) {
		setup := setupTest(t)
		runID := "run-no-drift"
		configFile := setup.absConfig(".rwx/sandbox.yml")
		configPath := configFile

		require.NoError(t, os.WriteFile(configPath, []byte("tasks:\n  - key: stable\n"), 0o644))
		stableHash := cli.HashConfigFile(configPath)

		seedSandboxStorageMulti(t, setup.tmp, map[string]cli.SandboxSession{
			"test-key": {
				RunID:      runID,
				ConfigFile: configFile,
				ConfigHash: stableHash,
			},
		})

		driftExecSetup(t, setup, runID)

		_, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: configFile,
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
		})

		require.NoError(t, err)
		require.NotContains(t, setup.mockStderr.String(), "has changed since this sandbox was started")
	})

	t.Run("does not warn when no stored hash exists", func(t *testing.T) {
		setup := setupTest(t)
		runID := "run-no-hash"
		configFile := setup.absConfig(".rwx/sandbox.yml")
		configPath := configFile

		require.NoError(t, os.WriteFile(configPath, []byte("tasks:\n  - key: test\n"), 0o644))

		seedSandboxStorageMulti(t, setup.tmp, map[string]cli.SandboxSession{
			"test-key": {
				RunID:      runID,
				ConfigFile: configFile,
			},
		})

		driftExecSetup(t, setup, runID)

		_, err := setup.service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile: configFile,
			Command:    []string{"echo", "hello"},
			RunID:      runID,
			Json:       true,
		})

		require.NoError(t, err)
		require.NotContains(t, setup.mockStderr.String(), "has changed since this sandbox was started")
	})
}
