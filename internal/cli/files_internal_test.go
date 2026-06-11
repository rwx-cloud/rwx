package cli

import (
	"os"
	"path/filepath"
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
