package text_test

import (
	"testing"

	"github.com/rwx-cloud/rwx/internal/text"
	"github.com/stretchr/testify/require"
)

func TestTruncate(t *testing.T) {
	t.Run("returns the string unchanged when it fits", func(t *testing.T) {
		require.Equal(t, "main", text.Truncate("main", 24))
		require.Equal(t, "main", text.Truncate("main", 4))
	})

	t.Run("truncates with a trailing ellipsis, staying within max", func(t *testing.T) {
		out := text.Truncate("a-very-long-branch-name", 10)
		require.Equal(t, "a-very-lo…", out)
		require.Equal(t, 10, len([]rune(out)))
	})

	t.Run("counts runes, not bytes", func(t *testing.T) {
		// Five 2-byte runes; a width of 5 fits, a width of 4 truncates to 4 runes.
		require.Equal(t, "héllo", text.Truncate("héllo", 5))
		require.Equal(t, "hél…", text.Truncate("héllo", 4))
	})

	t.Run("returns the string unchanged for a non-positive max", func(t *testing.T) {
		require.Equal(t, "anything", text.Truncate("anything", 0))
		require.Equal(t, "anything", text.Truncate("anything", -1))
	})
}
