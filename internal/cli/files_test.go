package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestReadRwxDirectoryEntries_SizeLimit(t *testing.T) {
	t.Run("returns patch-specific error when patch pushes total over 5MiB", func(t *testing.T) {
		t.Chdir(t.TempDir())

		rwxDir := ".rwx"
		err := os.MkdirAll(rwxDir, 0755)
		require.NoError(t, err)

		// Write a small config file (under 5MiB alone)
		smallContent := make([]byte, 1*1024*1024) // 1 MiB
		err = os.WriteFile(filepath.Join(rwxDir, "config.yml"), smallContent, 0644)
		require.NoError(t, err)

		// Write a large patch file that pushes total over 5MiB
		patchDir := filepath.Join(rwxDir, ".patches")
		err = os.MkdirAll(patchDir, 0755)
		require.NoError(t, err)

		largePatch := make([]byte, 5*1024*1024) // 5 MiB
		err = os.WriteFile(filepath.Join(patchDir, "abc123"), largePatch, 0644)
		require.NoError(t, err)

		_, err = cli.RwxDirectoryEntries(rwxDir)
		require.Error(t, err)
		require.Contains(t, err.Error(), "The .rwx directory is 1.0 MiB and the git patch for local changes is 5.0 MiB")
		require.Contains(t, err.Error(), "greater than the 5 MiB combined limit")
		require.Contains(t, err.Error(), "Push any unpushed commits")
	})

	t.Run("returns generic error when non-patch content exceeds 5MiB", func(t *testing.T) {
		t.Chdir(t.TempDir())

		rwxDir := ".rwx"
		err := os.MkdirAll(rwxDir, 0755)
		require.NoError(t, err)

		// Write large non-patch content that exceeds 5MiB on its own
		largeContent := make([]byte, 6*1024*1024) // 6 MiB
		err = os.WriteFile(filepath.Join(rwxDir, "big-config.yml"), largeContent, 0644)
		require.NoError(t, err)

		_, err = cli.RwxDirectoryEntries(rwxDir)
		require.Error(t, err)
		require.Contains(t, err.Error(), "the size of the these files exceed")
		require.NotContains(t, err.Error(), "git patch")
	})
}

func TestReadRwxDirectoryEntries_SkipsDownloads(t *testing.T) {
	t.Run("excludes downloads directory from size calculation", func(t *testing.T) {
		t.Chdir(t.TempDir())

		rwxDir := ".rwx"
		err := os.MkdirAll(rwxDir, 0755)
		require.NoError(t, err)

		// Write a small config file
		smallContent := make([]byte, 1*1024*1024) // 1 MiB
		err = os.WriteFile(filepath.Join(rwxDir, "config.yml"), smallContent, 0644)
		require.NoError(t, err)

		// Write a large file inside downloads that would push total over 5MiB
		downloadsDir := filepath.Join(rwxDir, "downloads")
		err = os.MkdirAll(downloadsDir, 0755)
		require.NoError(t, err)

		largeDownload := make([]byte, 6*1024*1024) // 6 MiB
		err = os.WriteFile(filepath.Join(downloadsDir, "artifact.tar.gz"), largeDownload, 0644)
		require.NoError(t, err)

		// Should succeed because downloads directory is excluded
		entries, err := cli.RwxDirectoryEntries(rwxDir)
		require.NoError(t, err)

		// Verify no entries from downloads directory are included
		for _, entry := range entries {
			require.NotContains(t, entry.Path, "downloads/")
		}
	})
}

func TestReadRwxDirectoryEntries_SkipsSandboxes(t *testing.T) {
	t.Run("excludes sandboxes directory from size calculation", func(t *testing.T) {
		t.Chdir(t.TempDir())

		rwxDir := ".rwx"
		err := os.MkdirAll(rwxDir, 0755)
		require.NoError(t, err)

		// Write a small config file
		smallContent := make([]byte, 1*1024*1024) // 1 MiB
		err = os.WriteFile(filepath.Join(rwxDir, "config.yml"), smallContent, 0644)
		require.NoError(t, err)

		// Write a large file inside sandboxes that would push total over 5MiB
		sandboxesDir := filepath.Join(rwxDir, "sandboxes")
		err = os.MkdirAll(sandboxesDir, 0755)
		require.NoError(t, err)

		largeSandbox := make([]byte, 6*1024*1024) // 6 MiB
		err = os.WriteFile(filepath.Join(sandboxesDir, "sandboxes.json"), largeSandbox, 0644)
		require.NoError(t, err)

		// Should succeed because sandboxes directory is excluded
		entries, err := cli.RwxDirectoryEntries(rwxDir)
		require.NoError(t, err)

		// Verify no entries from sandboxes directory are included
		for _, entry := range entries {
			require.NotContains(t, entry.Path, "sandboxes/")
		}
	})
}

func TestFindRunDefinitionFile(t *testing.T) {
	t.Run("when file exists in pwd", func(t *testing.T) {
		t.Chdir(t.TempDir())

		err := os.WriteFile("ci.yml", []byte{}, 0644)
		require.NoError(t, err)

		result, err := cli.FindRunDefinitionFile("ci.yml", "")
		require.NoError(t, err)
		require.Equal(t, "ci.yml", result)
	})

	t.Run("when file does not exist in pwd but exists in rwx directory", func(t *testing.T) {
		tmp := t.TempDir()
		t.Chdir(tmp)

		rwxDir := filepath.Join(tmp, ".rwx")
		err := os.Mkdir(rwxDir, 0755)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(rwxDir, "ci.yml"), []byte{}, 0644)
		require.NoError(t, err)

		result, err := cli.FindRunDefinitionFile("ci.yml", rwxDir)
		require.NoError(t, err)
		require.Equal(t, filepath.Join(rwxDir, "ci.yml"), result)
	})

	t.Run("when file exists in both pwd and rwx directory, prefers pwd", func(t *testing.T) {
		tmp := t.TempDir()
		t.Chdir(tmp)

		rwxDir := filepath.Join(tmp, ".rwx")
		err := os.Mkdir(rwxDir, 0755)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(tmp, "ci.yml"), []byte{}, 0644)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(rwxDir, "ci.yml"), []byte{}, 0644)
		require.NoError(t, err)

		result, err := cli.FindRunDefinitionFile("ci.yml", rwxDir)
		require.NoError(t, err)
		require.Equal(t, "ci.yml", result)
	})

	t.Run("when file does not exist in either location", func(t *testing.T) {
		tmp := t.TempDir()
		t.Chdir(tmp)

		rwxDir := filepath.Join(tmp, ".rwx")
		err := os.Mkdir(rwxDir, 0755)
		require.NoError(t, err)

		_, err = cli.FindRunDefinitionFile("nonexistent.yml", rwxDir)
		require.Error(t, err)
		require.Equal(t, `run definition file "nonexistent.yml" not found in current directory or in "`+rwxDir+`"`, err.Error())
	})

	t.Run("when file does not exist and no rwx directory provided", func(t *testing.T) {
		t.Chdir(t.TempDir())

		_, err := cli.FindRunDefinitionFile("nonexistent.yml", "")
		require.Error(t, err)
		require.Equal(t, `run definition file "nonexistent.yml" not found`, err.Error())
	})

	t.Run("when absolute path is provided and exists", func(t *testing.T) {
		tmp := t.TempDir()

		filePath := filepath.Join(tmp, "ci.yml")
		err := os.WriteFile(filePath, []byte{}, 0644)
		require.NoError(t, err)

		result, err := cli.FindRunDefinitionFile(filePath, "")
		require.NoError(t, err)
		require.Equal(t, filePath, result)
	})

	t.Run("when absolute path is provided and does not exist", func(t *testing.T) {
		tmp := t.TempDir()

		filePath := filepath.Join(tmp, "nonexistent.yml")

		_, err := cli.FindRunDefinitionFile(filePath, "")
		require.Error(t, err)
		require.Equal(t, `run definition file "`+filePath+`" not found`, err.Error())
	})
}

func TestFindDefaultDownloadsDir(t *testing.T) {
	// getWd returns the working directory as os.Getwd reports it, matching
	// what findRwxDirectoryPath uses internally.
	getWd := func(t *testing.T) string {
		t.Helper()
		wd, err := os.Getwd()
		require.NoError(t, err)
		return wd
	}

	t.Run("returns .rwx/downloads when .rwx directory exists", func(t *testing.T) {
		t.Chdir(t.TempDir())
		wd := getWd(t)

		err := os.Mkdir(filepath.Join(wd, ".rwx"), 0755)
		require.NoError(t, err)

		result, err := cli.FindDefaultDownloadsDir()
		require.NoError(t, err)

		expectedDownloadsDir := filepath.Join(wd, ".rwx", "downloads")
		require.Equal(t, expectedDownloadsDir, result)

		// Verify directory was created
		info, err := os.Stat(expectedDownloadsDir)
		require.NoError(t, err)
		require.True(t, info.IsDir())
	})

	t.Run("returns .mint/downloads when .mint directory exists", func(t *testing.T) {
		t.Chdir(t.TempDir())
		wd := getWd(t)

		err := os.Mkdir(filepath.Join(wd, ".mint"), 0755)
		require.NoError(t, err)

		result, err := cli.FindDefaultDownloadsDir()
		require.NoError(t, err)

		expectedDownloadsDir := filepath.Join(wd, ".mint", "downloads")
		require.Equal(t, expectedDownloadsDir, result)
	})

	t.Run("falls back to ~/Downloads when no .rwx directory exists", func(t *testing.T) {
		t.Chdir(t.TempDir())

		result, err := cli.FindDefaultDownloadsDir()
		require.NoError(t, err)

		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)
		require.Equal(t, filepath.Join(homeDir, "Downloads"), result)
	})

	t.Run("creates .gitignore with wildcard inside downloads directory", func(t *testing.T) {
		t.Chdir(t.TempDir())
		wd := getWd(t)

		err := os.Mkdir(filepath.Join(wd, ".rwx"), 0755)
		require.NoError(t, err)

		result, err := cli.FindDefaultDownloadsDir()
		require.NoError(t, err)

		gitignorePath := filepath.Join(result, ".gitignore")
		content, err := os.ReadFile(gitignorePath)
		require.NoError(t, err)
		require.Equal(t, "*\n", string(content))
	})

	t.Run("does not overwrite existing .gitignore in downloads directory", func(t *testing.T) {
		t.Chdir(t.TempDir())
		wd := getWd(t)

		downloadsDir := filepath.Join(wd, ".rwx", "downloads")
		err := os.MkdirAll(downloadsDir, 0755)
		require.NoError(t, err)

		// Write a custom .gitignore before calling FindDefaultDownloadsDir
		gitignorePath := filepath.Join(downloadsDir, ".gitignore")
		err = os.WriteFile(gitignorePath, []byte("custom\n"), 0644)
		require.NoError(t, err)

		_, err = cli.FindDefaultDownloadsDir()
		require.NoError(t, err)

		content, err := os.ReadFile(gitignorePath)
		require.NoError(t, err)
		require.Equal(t, "custom\n", string(content))
	})
}
