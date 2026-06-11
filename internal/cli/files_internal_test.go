package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNextParentDirectory(t *testing.T) {
	t.Run("returns parent for nested directory", func(t *testing.T) {
		parent, ok := nextParentDirectory(filepath.Join("tmp", "project", "subdir"))
		require.True(t, ok)
		require.Equal(t, filepath.Join("tmp", "project"), parent)
	})

	t.Run("stops when parent is the same directory", func(t *testing.T) {
		parent, ok := nextParentDirectory(filepath.Clean(string(os.PathSeparator)))
		require.False(t, ok)
		require.Empty(t, parent)
	})
}

func TestRwxDirectoryEntries(t *testing.T) {
	t.Run("returns walk errors", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("directory permissions are not enforced the same way on Windows")
		}

		tmpDir := t.TempDir()
		blockedDir := filepath.Join(tmpDir, "blocked")
		require.NoError(t, os.Mkdir(blockedDir, 0o755))
		require.NoError(t, os.Chmod(blockedDir, 0o000))
		t.Cleanup(func() { _ = os.Chmod(blockedDir, 0o755) })

		_, err := rwxDirectoryEntries(tmpDir)
		require.Error(t, err)
		require.Contains(t, err.Error(), "reading rwx directory entries")
	})
}
