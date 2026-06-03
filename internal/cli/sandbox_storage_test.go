package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestSessionKey(t *testing.T) {
	t.Run("creates key from branch and config file", func(t *testing.T) {
		key := cli.SessionKey("main", "/home/user/project/.rwx/sandbox.yml")
		require.Equal(t, "main:/home/user/project/.rwx/sandbox.yml", key)
	})

	t.Run("uses 'detached' when branch is empty", func(t *testing.T) {
		key := cli.SessionKey("", "/home/user/project/.rwx/sandbox.yml")
		require.Equal(t, "detached:/home/user/project/.rwx/sandbox.yml", key)
	})

	t.Run("preserves detached@sha format as-is", func(t *testing.T) {
		key := cli.SessionKey("detached@abc1234", "/home/user/project/.rwx/sandbox.yml")
		require.Equal(t, "detached@abc1234:/home/user/project/.rwx/sandbox.yml", key)
	})
}

func TestParseSessionKey(t *testing.T) {
	t.Run("parses standard key with absolute config path", func(t *testing.T) {
		branch, configFile := cli.ParseSessionKey("main:/home/user/project/.rwx/sandbox.yml")
		require.Equal(t, "main", branch)
		require.Equal(t, "/home/user/project/.rwx/sandbox.yml", configFile)
	})

	t.Run("parses key with detached branch", func(t *testing.T) {
		branch, configFile := cli.ParseSessionKey("detached:/home/user/project/.rwx/sandbox.yml")
		require.Equal(t, "detached", branch)
		require.Equal(t, "/home/user/project/.rwx/sandbox.yml", configFile)
	})

	t.Run("parses key with detached@sha branch", func(t *testing.T) {
		branch, configFile := cli.ParseSessionKey("detached@abc1234:/home/user/project/.rwx/sandbox.yml")
		require.Equal(t, "detached@abc1234", branch)
		require.Equal(t, "/home/user/project/.rwx/sandbox.yml", configFile)
	})

	t.Run("parses key with feature branch containing slashes", func(t *testing.T) {
		branch, configFile := cli.ParseSessionKey("feature/test:/home/user/project/.rwx/sandbox.yml")
		require.Equal(t, "feature/test", branch)
		require.Equal(t, "/home/user/project/.rwx/sandbox.yml", configFile)
	})

	t.Run("round-trips with SessionKey", func(t *testing.T) {
		originalBranch := "feature/new-feature"
		originalConfig := "/home/user/project/.rwx/sandbox.yml"

		key := cli.SessionKey(originalBranch, originalConfig)
		branch, configFile := cli.ParseSessionKey(key)

		require.Equal(t, originalBranch, branch)
		require.Equal(t, originalConfig, configFile)
	})

	t.Run("handles key with no colons", func(t *testing.T) {
		branch, configFile := cli.ParseSessionKey("invalid")
		require.Equal(t, "invalid", branch)
		require.Equal(t, "", configFile)
	})

	t.Run("handles relative config file fallback", func(t *testing.T) {
		branch, configFile := cli.ParseSessionKey("main:.rwx/sandbox.yml")
		require.Equal(t, "main", branch)
		require.Equal(t, ".rwx/sandbox.yml", configFile)
	})
}

func TestSandboxStorage_SessionOperations(t *testing.T) {
	t.Run("SetSession and GetSession", func(t *testing.T) {
		storage := &cli.SandboxStorage{
			Sandboxes: make(map[string]cli.SandboxSession),
		}

		session := cli.SandboxSession{
			RunID:      "run-123",
			ConfigFile: "/home/user/project/.rwx/sandbox.yml",
		}

		storage.SetSession("main", "/home/user/project/.rwx/sandbox.yml", session)

		retrieved, found := storage.GetSession("main", "/home/user/project/.rwx/sandbox.yml")
		require.True(t, found)
		require.Equal(t, "run-123", retrieved.RunID)
		require.Equal(t, "/home/user/project/.rwx/sandbox.yml", retrieved.ConfigFile)
	})

	t.Run("SetSession and GetSession with ScopedToken", func(t *testing.T) {
		storage := &cli.SandboxStorage{
			Sandboxes: make(map[string]cli.SandboxSession),
		}

		session := cli.SandboxSession{
			RunID:       "run-123",
			ConfigFile:  "/home/user/project/.rwx/sandbox.yml",
			ScopedToken: "scoped-token-abc",
		}

		storage.SetSession("main", "/home/user/project/.rwx/sandbox.yml", session)

		retrieved, found := storage.GetSession("main", "/home/user/project/.rwx/sandbox.yml")
		require.True(t, found)
		require.Equal(t, "run-123", retrieved.RunID)
		require.Equal(t, "/home/user/project/.rwx/sandbox.yml", retrieved.ConfigFile)
		require.Equal(t, "scoped-token-abc", retrieved.ScopedToken)
	})

	t.Run("GetSession returns false when not found", func(t *testing.T) {
		storage := &cli.SandboxStorage{
			Sandboxes: make(map[string]cli.SandboxSession),
		}

		_, found := storage.GetSession("main", "/home/user/project/.rwx/sandbox.yml")
		require.False(t, found)
	})

	t.Run("DeleteSession removes session", func(t *testing.T) {
		storage := &cli.SandboxStorage{
			Sandboxes: make(map[string]cli.SandboxSession),
		}

		session := cli.SandboxSession{RunID: "run-123", ConfigFile: "/home/user/project/.rwx/sandbox.yml"}
		storage.SetSession("main", "/home/user/project/.rwx/sandbox.yml", session)

		storage.DeleteSession("main", "/home/user/project/.rwx/sandbox.yml")

		_, found := storage.GetSession("main", "/home/user/project/.rwx/sandbox.yml")
		require.False(t, found)
	})

	t.Run("DeleteSession is no-op when session does not exist", func(t *testing.T) {
		storage := &cli.SandboxStorage{
			Sandboxes: make(map[string]cli.SandboxSession),
		}

		// Should not panic
		storage.DeleteSession("main", "/home/user/project/.rwx/sandbox.yml")
		require.Empty(t, storage.Sandboxes)
	})
}

func TestSandboxStorage_GetSessionsForBranch(t *testing.T) {
	t.Run("returns all sessions matching branch", func(t *testing.T) {
		storage := &cli.SandboxStorage{
			Sandboxes: make(map[string]cli.SandboxSession),
		}

		storage.SetSession("main", "/project/.rwx/config1.yml", cli.SandboxSession{RunID: "run-1"})
		storage.SetSession("main", "/project/.rwx/config2.yml", cli.SandboxSession{RunID: "run-2"})
		storage.SetSession("develop", "/project/.rwx/config1.yml", cli.SandboxSession{RunID: "run-3"})

		sessions := storage.GetSessionsForBranch("main")
		require.Len(t, sessions, 2)

		runIDs := make([]string, len(sessions))
		for i, s := range sessions {
			runIDs[i] = s.RunID
		}
		require.ElementsMatch(t, []string{"run-1", "run-2"}, runIDs)
	})

	t.Run("returns empty slice when no matches", func(t *testing.T) {
		storage := &cli.SandboxStorage{
			Sandboxes: make(map[string]cli.SandboxSession),
		}

		storage.SetSession("main", "/project/.rwx/config.yml", cli.SandboxSession{RunID: "run-1"})

		sessions := storage.GetSessionsForBranch("develop")
		require.Empty(t, sessions)
	})

	t.Run("handles detached branch", func(t *testing.T) {
		storage := &cli.SandboxStorage{
			Sandboxes: make(map[string]cli.SandboxSession),
		}

		storage.SetSession("", "/project/.rwx/config.yml", cli.SandboxSession{RunID: "run-1"})

		sessions := storage.GetSessionsForBranch("")
		require.Len(t, sessions, 1)
		require.Equal(t, "run-1", sessions[0].RunID)
	})
}

func TestSandboxStorage_FindByRunID(t *testing.T) {
	t.Run("finds session by run ID", func(t *testing.T) {
		storage := &cli.SandboxStorage{
			Sandboxes: make(map[string]cli.SandboxSession),
		}

		storage.SetSession("main", "/project/.rwx/config.yml", cli.SandboxSession{
			RunID:      "run-123",
			ConfigFile: "/project/.rwx/config.yml",
		})

		session, key, found := storage.FindByRunID("run-123")
		require.True(t, found)
		require.Equal(t, "run-123", session.RunID)
		require.Equal(t, "main:/project/.rwx/config.yml", key)
	})

	t.Run("returns false when run ID not found", func(t *testing.T) {
		storage := &cli.SandboxStorage{
			Sandboxes: make(map[string]cli.SandboxSession),
		}

		storage.SetSession("main", "/project/.rwx/config.yml", cli.SandboxSession{RunID: "run-123"})

		_, _, found := storage.FindByRunID("run-456")
		require.False(t, found)
	})
}

func TestSandboxStorage_DeleteSessionByRunID(t *testing.T) {
	t.Run("deletes session and returns true", func(t *testing.T) {
		storage := &cli.SandboxStorage{
			Sandboxes: make(map[string]cli.SandboxSession),
		}

		storage.SetSession("main", "/project/.rwx/config.yml", cli.SandboxSession{RunID: "run-123"})

		deleted := storage.DeleteSessionByRunID("run-123")
		require.True(t, deleted)

		_, found := storage.GetSession("main", "/project/.rwx/config.yml")
		require.False(t, found)
	})

	t.Run("returns false when run ID not found", func(t *testing.T) {
		storage := &cli.SandboxStorage{
			Sandboxes: make(map[string]cli.SandboxSession),
		}

		deleted := storage.DeleteSessionByRunID("run-456")
		require.False(t, deleted)
	})
}

func TestSandboxStorage_AllSessions(t *testing.T) {
	t.Run("returns all sessions", func(t *testing.T) {
		storage := &cli.SandboxStorage{
			Sandboxes: make(map[string]cli.SandboxSession),
		}

		storage.SetSession("main", "/project1/.rwx/config.yml", cli.SandboxSession{RunID: "run-1"})
		storage.SetSession("develop", "/project2/.rwx/config.yml", cli.SandboxSession{RunID: "run-2"})

		all := storage.AllSessions()
		require.Len(t, all, 2)
	})

	t.Run("returns empty map when no sessions", func(t *testing.T) {
		storage := &cli.SandboxStorage{
			Sandboxes: make(map[string]cli.SandboxSession),
		}

		all := storage.AllSessions()
		require.Empty(t, all)
	})
}

// setupTestStorageDir creates a temp dir with a .rwx directory, sets HOME to it,
// and chdirs into it.
func setupTestStorageDir(t *testing.T) string {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "sandbox-storage-test")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", originalHome) })

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() { _ = os.Chdir(originalWd) })

	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, ".rwx"), 0o755))

	return tmpDir
}

func TestSandboxStorage_LoadAndSave(t *testing.T) {
	t.Run("returns empty storage when file does not exist", func(t *testing.T) {
		setupTestStorageDir(t)

		storage, err := cli.LoadSandboxStorage()
		require.NoError(t, err)
		require.NotNil(t, storage)
		require.Empty(t, storage.Sandboxes)
	})

	t.Run("saves and loads from local rwx path", func(t *testing.T) {
		tmpDir := setupTestStorageDir(t)

		storage := &cli.SandboxStorage{
			Sandboxes: make(map[string]cli.SandboxSession),
		}
		storage.SetSession("main", "/home/user/project/.rwx/sandbox.yml", cli.SandboxSession{
			RunID:      "run-123",
			ConfigFile: "/home/user/project/.rwx/sandbox.yml",
		})

		err := storage.Save()
		require.NoError(t, err)

		localPath := filepath.Join(tmpDir, ".rwx", "sandboxes", "sandboxes.json")
		_, err = os.Stat(localPath)
		require.NoError(t, err)

		loaded, err := cli.LoadSandboxStorage()
		require.NoError(t, err)
		require.Len(t, loaded.Sandboxes, 1)

		session, found := loaded.GetSession("main", "/home/user/project/.rwx/sandbox.yml")
		require.True(t, found)
		require.Equal(t, "run-123", session.RunID)
	})

	t.Run("creates .gitignore in sandboxes dir", func(t *testing.T) {
		tmpDir := setupTestStorageDir(t)

		storage := &cli.SandboxStorage{
			Sandboxes: make(map[string]cli.SandboxSession),
		}
		err := storage.Save()
		require.NoError(t, err)

		gitignorePath := filepath.Join(tmpDir, ".rwx", "sandboxes", ".gitignore")
		contents, err := os.ReadFile(gitignorePath)
		require.NoError(t, err)
		require.Equal(t, "*\n", string(contents))
	})

	t.Run("creates .gitignore before locking storage", func(t *testing.T) {
		tmpDir := setupTestStorageDir(t)

		lock, err := cli.LockSandboxStorage()
		require.NoError(t, err)
		defer cli.UnlockSandboxStorage(lock)

		gitignorePath := filepath.Join(tmpDir, ".rwx", "sandboxes", ".gitignore")
		contents, err := os.ReadFile(gitignorePath)
		require.NoError(t, err)
		require.Equal(t, "*\n", string(contents))
	})

	t.Run("creates directory structure if it does not exist", func(t *testing.T) {
		tmpDir := setupTestStorageDir(t)

		storage := &cli.SandboxStorage{
			Sandboxes: make(map[string]cli.SandboxSession),
		}

		err := storage.Save()
		require.NoError(t, err)

		sandboxesDir := filepath.Join(tmpDir, ".rwx", "sandboxes")
		info, err := os.Stat(sandboxesDir)
		require.NoError(t, err)
		require.True(t, info.IsDir())
	})

	t.Run("handles nil Sandboxes map in stored file", func(t *testing.T) {
		tmpDir := setupTestStorageDir(t)

		sandboxesDir := filepath.Join(tmpDir, ".rwx", "sandboxes")
		require.NoError(t, os.MkdirAll(sandboxesDir, 0o755))
		storagePath := filepath.Join(sandboxesDir, "sandboxes.json")
		require.NoError(t, os.WriteFile(storagePath, []byte(`{"sandboxes": null}`), 0o644))

		storage, err := cli.LoadSandboxStorage()
		require.NoError(t, err)
		require.NotNil(t, storage.Sandboxes)
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		tmpDir := setupTestStorageDir(t)

		sandboxesDir := filepath.Join(tmpDir, ".rwx", "sandboxes")
		require.NoError(t, os.MkdirAll(sandboxesDir, 0o755))
		storagePath := filepath.Join(sandboxesDir, "sandboxes.json")
		require.NoError(t, os.WriteFile(storagePath, []byte(`{invalid json`), 0o644))

		_, err := cli.LoadSandboxStorage()
		require.Error(t, err)
		require.Contains(t, err.Error(), "unable to parse")
	})

	t.Run("migrates old-format keys on load", func(t *testing.T) {
		tmpDir := setupTestStorageDir(t)

		sandboxesDir := filepath.Join(tmpDir, ".rwx", "sandboxes")
		require.NoError(t, os.MkdirAll(sandboxesDir, 0o755))
		storagePath := filepath.Join(sandboxesDir, "sandboxes.json")
		// Old format: cwd:branch:configFile (no version field)
		oldJSON := `{"sandboxes":{"/home/user/project:main:/home/user/project/.rwx/sandbox.yml":{"runId":"run-old","configFile":"/home/user/project/.rwx/sandbox.yml"}}}`
		require.NoError(t, os.WriteFile(storagePath, []byte(oldJSON), 0o644))

		storage, err := cli.LoadSandboxStorage()
		require.NoError(t, err)
		require.Len(t, storage.Sandboxes, 1)

		// Should be accessible via new-format key
		session, found := storage.GetSession("main", "/home/user/project/.rwx/sandbox.yml")
		require.True(t, found)
		require.Equal(t, "run-old", session.RunID)
	})

	t.Run("skips migration when version is current", func(t *testing.T) {
		tmpDir := setupTestStorageDir(t)

		sandboxesDir := filepath.Join(tmpDir, ".rwx", "sandboxes")
		require.NoError(t, os.MkdirAll(sandboxesDir, 0o755))
		storagePath := filepath.Join(sandboxesDir, "sandboxes.json")
		// Current version with a key that looks like it could be old-format but shouldn't be migrated
		currentJSON := `{"version":1,"sandboxes":{"main:/home/user/project/.rwx/sandbox.yml":{"runId":"run-current","configFile":"/home/user/project/.rwx/sandbox.yml"}}}`
		require.NoError(t, os.WriteFile(storagePath, []byte(currentJSON), 0o644))

		storage, err := cli.LoadSandboxStorage()
		require.NoError(t, err)
		require.Len(t, storage.Sandboxes, 1)

		session, found := storage.GetSession("main", "/home/user/project/.rwx/sandbox.yml")
		require.True(t, found)
		require.Equal(t, "run-current", session.RunID)
	})
}

func TestEncodeDecodeCliState(t *testing.T) {
	t.Run("round-trips correctly", func(t *testing.T) {
		encoded := cli.EncodeCliState("main", "/home/user/project/.rwx/sandbox.yml")
		state, err := cli.DecodeCliState(encoded)
		require.NoError(t, err)
		require.Equal(t, "main", state.Branch)
		require.Equal(t, "/home/user/project/.rwx/sandbox.yml", state.ConfigFile)
	})

	t.Run("handles empty fields", func(t *testing.T) {
		encoded := cli.EncodeCliState("", "")
		state, err := cli.DecodeCliState(encoded)
		require.NoError(t, err)
		require.Equal(t, "", state.Branch)
		require.Equal(t, "", state.ConfigFile)
	})

	t.Run("handles special characters", func(t *testing.T) {
		encoded := cli.EncodeCliState("feature/test-branch", "/path/with spaces/config.yml")
		state, err := cli.DecodeCliState(encoded)
		require.NoError(t, err)
		require.Equal(t, "feature/test-branch", state.Branch)
		require.Equal(t, "/path/with spaces/config.yml", state.ConfigFile)
	})

	t.Run("decodes old format with cwd gracefully", func(t *testing.T) {
		// Old CliState payloads include "cwd" — the field is silently ignored
		encoded := cli.EncodeCliState("main", "/project/.rwx/sandbox.yml")
		state, err := cli.DecodeCliState(encoded)
		require.NoError(t, err)
		require.Equal(t, "main", state.Branch)
		require.Equal(t, "/project/.rwx/sandbox.yml", state.ConfigFile)
	})

	t.Run("returns error for invalid base64", func(t *testing.T) {
		_, err := cli.DecodeCliState("not-valid-base64!!!")
		require.Error(t, err)
		require.Contains(t, err.Error(), "unable to decode cli_state")
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		// Valid base64 but not valid JSON
		encoded := "bm90LWpzb24==" // "not-json"
		_, err := cli.DecodeCliState(encoded)
		require.Error(t, err)
	})
}

func TestSandboxTitle(t *testing.T) {
	t.Run("creates title from project name and branch", func(t *testing.T) {
		title := cli.SandboxTitle("/home/user/my-project", "main", ".rwx/sandbox.yml")
		require.Equal(t, "Sandbox: my-project (main)", title)
	})

	t.Run("uses 'detached' when branch is empty", func(t *testing.T) {
		title := cli.SandboxTitle("/home/user/my-project", "", ".rwx/sandbox.yml")
		require.Equal(t, "Sandbox: my-project (detached)", title)
	})

	t.Run("displays detached with short SHA", func(t *testing.T) {
		title := cli.SandboxTitle("/home/user/my-project", "detached@abc1234", ".rwx/sandbox.yml")
		require.Equal(t, "Sandbox: my-project (detached abc1234)", title)
	})

	t.Run("includes non-default config file", func(t *testing.T) {
		title := cli.SandboxTitle("/home/user/my-project", "feature/test", ".rwx/custom.yml")
		require.Equal(t, "Sandbox: my-project (feature/test) [.rwx/custom.yml]", title)
	})

	t.Run("excludes default config file", func(t *testing.T) {
		title := cli.SandboxTitle("/home/user/my-project", "develop", ".rwx/sandbox.yml")
		require.Equal(t, "Sandbox: my-project (develop)", title)
	})

	t.Run("handles empty config file", func(t *testing.T) {
		title := cli.SandboxTitle("/home/user/my-project", "main", "")
		require.Equal(t, "Sandbox: my-project (main)", title)
	})
}

func TestIsDetachedBranch(t *testing.T) {
	t.Run("returns true for bare 'detached'", func(t *testing.T) {
		require.True(t, cli.IsDetachedBranch("detached"))
	})

	t.Run("returns true for 'detached@<sha>'", func(t *testing.T) {
		require.True(t, cli.IsDetachedBranch("detached@abc1234"))
	})

	t.Run("returns false for regular branch", func(t *testing.T) {
		require.False(t, cli.IsDetachedBranch("main"))
	})

	t.Run("returns false for empty string", func(t *testing.T) {
		require.False(t, cli.IsDetachedBranch(""))
	})
}

func TestDetachedShortSHA(t *testing.T) {
	t.Run("extracts SHA from detached@<sha>", func(t *testing.T) {
		require.Equal(t, "abc1234", cli.DetachedShortSHA("detached@abc1234"))
	})

	t.Run("returns empty for bare 'detached'", func(t *testing.T) {
		require.Equal(t, "", cli.DetachedShortSHA("detached"))
	})

	t.Run("returns empty for regular branch", func(t *testing.T) {
		require.Equal(t, "", cli.DetachedShortSHA("main"))
	})
}

func TestHashConfigFile(t *testing.T) {
	t.Run("returns consistent hash for same content", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "sandbox.yml")
		require.NoError(t, os.WriteFile(tmpFile, []byte("tasks:\n  - key: test\n"), 0o644))

		hash1 := cli.HashConfigFile(tmpFile)
		hash2 := cli.HashConfigFile(tmpFile)
		require.NotEmpty(t, hash1)
		require.Equal(t, hash1, hash2)
	})

	t.Run("returns different hash for different content", func(t *testing.T) {
		dir := t.TempDir()
		file1 := filepath.Join(dir, "a.yml")
		file2 := filepath.Join(dir, "b.yml")
		require.NoError(t, os.WriteFile(file1, []byte("version: 1"), 0o644))
		require.NoError(t, os.WriteFile(file2, []byte("version: 2"), 0o644))

		require.NotEqual(t, cli.HashConfigFile(file1), cli.HashConfigFile(file2))
	})

	t.Run("returns empty string for nonexistent file", func(t *testing.T) {
		require.Equal(t, "", cli.HashConfigFile("/nonexistent/file.yml"))
	})
}

// mockAncestryChecker implements cli.AncestryChecker for testing.
type mockAncestryChecker struct {
	ancestors map[string]bool // key: "candidate->head"
}

func (m *mockAncestryChecker) IsAncestor(candidateSHA, headRef string) bool {
	return m.ancestors[candidateSHA+"->"+headRef]
}

func TestGetSessionByAncestry(t *testing.T) {
	t.Run("returns nil when branch is not detached", func(t *testing.T) {
		storage := &cli.SandboxStorage{Sandboxes: make(map[string]cli.SandboxSession)}
		storage.SetSession("detached@abc1234", "/project/.rwx/config.yml", cli.SandboxSession{RunID: "run-1"})

		checker := &mockAncestryChecker{ancestors: map[string]bool{"abc1234->HEAD": true}}
		session, found := storage.GetSessionByAncestry("main", "/project/.rwx/config.yml", checker)
		require.False(t, found)
		require.Nil(t, session)
	})

	t.Run("returns nil when branch is bare detached without SHA", func(t *testing.T) {
		storage := &cli.SandboxStorage{Sandboxes: make(map[string]cli.SandboxSession)}
		storage.SetSession("detached@abc1234", "/project/.rwx/config.yml", cli.SandboxSession{RunID: "run-1"})

		checker := &mockAncestryChecker{ancestors: map[string]bool{"abc1234->HEAD": true}}
		session, found := storage.GetSessionByAncestry("detached", "/project/.rwx/config.yml", checker)
		require.False(t, found)
		require.Nil(t, session)
	})

	t.Run("finds session when stored SHA is ancestor of HEAD", func(t *testing.T) {
		storage := &cli.SandboxStorage{Sandboxes: make(map[string]cli.SandboxSession)}
		storage.SetSession("detached@abc1234", "/project/.rwx/config.yml", cli.SandboxSession{RunID: "run-1", ConfigFile: "/project/.rwx/config.yml"})

		checker := &mockAncestryChecker{ancestors: map[string]bool{"abc1234->HEAD": true}}
		session, found := storage.GetSessionByAncestry("detached@def5678", "/project/.rwx/config.yml", checker)
		require.True(t, found)
		require.Equal(t, "run-1", session.RunID)

		// Verify key was updated
		_, oldFound := storage.GetSession("detached@abc1234", "/project/.rwx/config.yml")
		require.False(t, oldFound)
		newSession, newFound := storage.GetSession("detached@def5678", "/project/.rwx/config.yml")
		require.True(t, newFound)
		require.Equal(t, "run-1", newSession.RunID)
	})

	t.Run("does not match when stored SHA is not an ancestor", func(t *testing.T) {
		storage := &cli.SandboxStorage{Sandboxes: make(map[string]cli.SandboxSession)}
		storage.SetSession("detached@abc1234", "/project/.rwx/config.yml", cli.SandboxSession{RunID: "run-1"})

		checker := &mockAncestryChecker{ancestors: map[string]bool{}}
		session, found := storage.GetSessionByAncestry("detached@def5678", "/project/.rwx/config.yml", checker)
		require.False(t, found)
		require.Nil(t, session)

		// Verify original key is untouched
		_, stillThere := storage.GetSession("detached@abc1234", "/project/.rwx/config.yml")
		require.True(t, stillThere)
	})

	t.Run("does not match sessions with different config file", func(t *testing.T) {
		storage := &cli.SandboxStorage{Sandboxes: make(map[string]cli.SandboxSession)}
		storage.SetSession("detached@abc1234", "/project/.rwx/other.yml", cli.SandboxSession{RunID: "run-1"})

		checker := &mockAncestryChecker{ancestors: map[string]bool{"abc1234->HEAD": true}}
		session, found := storage.GetSessionByAncestry("detached@def5678", "/project/.rwx/config.yml", checker)
		require.False(t, found)
		require.Nil(t, session)
	})

	t.Run("does not match sessions stored under a named branch", func(t *testing.T) {
		storage := &cli.SandboxStorage{Sandboxes: make(map[string]cli.SandboxSession)}
		storage.SetSession("main", "/project/.rwx/config.yml", cli.SandboxSession{RunID: "run-1"})

		checker := &mockAncestryChecker{ancestors: map[string]bool{}}
		session, found := storage.GetSessionByAncestry("detached@def5678", "/project/.rwx/config.yml", checker)
		require.False(t, found)
		require.Nil(t, session)
	})

	t.Run("does not match bare detached stored sessions", func(t *testing.T) {
		storage := &cli.SandboxStorage{Sandboxes: make(map[string]cli.SandboxSession)}
		storage.SetSession("detached", "/project/.rwx/config.yml", cli.SandboxSession{RunID: "run-1"})

		checker := &mockAncestryChecker{ancestors: map[string]bool{}}
		session, found := storage.GetSessionByAncestry("detached@def5678", "/project/.rwx/config.yml", checker)
		require.False(t, found)
		require.Nil(t, session)
	})
}

func TestGetSessionsForBranchByAncestry(t *testing.T) {
	t.Run("returns nil when branch is not detached", func(t *testing.T) {
		storage := &cli.SandboxStorage{Sandboxes: make(map[string]cli.SandboxSession)}
		storage.SetSession("detached@abc1234", "/project/.rwx/config.yml", cli.SandboxSession{RunID: "run-1"})

		checker := &mockAncestryChecker{ancestors: map[string]bool{"abc1234->HEAD": true}}
		sessions := storage.GetSessionsForBranchByAncestry("main", checker)
		require.Nil(t, sessions)
	})

	t.Run("returns matching sessions across multiple configs", func(t *testing.T) {
		storage := &cli.SandboxStorage{Sandboxes: make(map[string]cli.SandboxSession)}
		storage.SetSession("detached@abc1234", "/project/.rwx/config1.yml", cli.SandboxSession{RunID: "run-1"})
		storage.SetSession("detached@abc1234", "/project/.rwx/config2.yml", cli.SandboxSession{RunID: "run-2"})

		checker := &mockAncestryChecker{ancestors: map[string]bool{"abc1234->HEAD": true}}
		sessions := storage.GetSessionsForBranchByAncestry("detached@def5678", checker)
		require.Len(t, sessions, 2)

		runIDs := []string{sessions[0].RunID, sessions[1].RunID}
		require.ElementsMatch(t, []string{"run-1", "run-2"}, runIDs)

		// Verify keys were updated
		_, oldFound := storage.GetSession("detached@abc1234", "/project/.rwx/config1.yml")
		require.False(t, oldFound)
		_, newFound := storage.GetSession("detached@def5678", "/project/.rwx/config1.yml")
		require.True(t, newFound)
	})

	t.Run("does not return non-ancestor detached sessions", func(t *testing.T) {
		storage := &cli.SandboxStorage{Sandboxes: make(map[string]cli.SandboxSession)}
		storage.SetSession("detached@abc1234", "/project/.rwx/config.yml", cli.SandboxSession{RunID: "run-1"})
		storage.SetSession("detached@unrelated", "/project/.rwx/config.yml", cli.SandboxSession{RunID: "run-2"})

		checker := &mockAncestryChecker{ancestors: map[string]bool{"abc1234->HEAD": true}}
		sessions := storage.GetSessionsForBranchByAncestry("detached@def5678", checker)
		require.Len(t, sessions, 1)
		require.Equal(t, "run-1", sessions[0].RunID)
	})

	t.Run("returns empty when no matches", func(t *testing.T) {
		storage := &cli.SandboxStorage{Sandboxes: make(map[string]cli.SandboxSession)}
		storage.SetSession("detached@abc1234", "/project/.rwx/config.yml", cli.SandboxSession{RunID: "run-1"})

		checker := &mockAncestryChecker{ancestors: map[string]bool{}}
		sessions := storage.GetSessionsForBranchByAncestry("detached@def5678", checker)
		require.Empty(t, sessions)
	})
}

func TestGetSessionsForBranch_DetachedSHA(t *testing.T) {
	t.Run("different detached SHAs get separate sessions", func(t *testing.T) {
		storage := &cli.SandboxStorage{
			Sandboxes: make(map[string]cli.SandboxSession),
		}

		storage.SetSession("detached@abc1234", "/project/.rwx/config.yml", cli.SandboxSession{RunID: "run-1"})
		storage.SetSession("detached@def5678", "/project/.rwx/config.yml", cli.SandboxSession{RunID: "run-2"})

		sessions1 := storage.GetSessionsForBranch("detached@abc1234")
		require.Len(t, sessions1, 1)
		require.Equal(t, "run-1", sessions1[0].RunID)

		sessions2 := storage.GetSessionsForBranch("detached@def5678")
		require.Len(t, sessions2, 1)
		require.Equal(t, "run-2", sessions2[0].RunID)
	})
}
