package cli_test

import (
	"os"
	"testing"

	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

// unsetTerminalEnv removes every variable Hyperlink reads, so the host terminal
// cannot change the result of a test.
func unsetTerminalEnv(t *testing.T) {
	for _, name := range []string{
		"FORCE_HYPERLINK",
		"NO_COLOR",
		"VTE_VERSION",
		"TERM_PROGRAM",
		"TERM_PROGRAM_VERSION",
		"TERM",
		"COLORTERM",
		"TERMINAL_EMULATOR",
		"DOMTERM",
		"WT_SESSION",
		"KONSOLE_VERSION",
	} {
		original, existed := os.LookupEnv(name)
		require.NoError(t, os.Unsetenv(name))
		if existed {
			t.Cleanup(func() {
				require.NoError(t, os.Setenv(name, original))
			})
		}
	}
}

func TestHyperlink(t *testing.T) {
	url := "https://cloud.rwx.com/_/personal_access_tokens/123/edit"

	t.Run("when stdout is not a TTY", func(t *testing.T) {
		unsetTerminalEnv(t)
		t.Setenv("FORCE_HYPERLINK", "1")

		require.False(t, cli.HyperlinksSupported(false))
		require.Equal(t, url, cli.Hyperlink("Edit token", url, false))
	})

	t.Run("when the terminal does not support hyperlinks", func(t *testing.T) {
		unsetTerminalEnv(t)
		t.Setenv("TERM", "dumb")
		t.Setenv("TERM_PROGRAM", "Apple_Terminal")
		t.Setenv("TERM_PROGRAM_VERSION", "455")

		require.False(t, cli.HyperlinksSupported(true))
		require.Equal(t, url, cli.Hyperlink("Edit token", url, true))
	})

	t.Run("when the terminal supports hyperlinks", func(t *testing.T) {
		unsetTerminalEnv(t)
		t.Setenv("FORCE_HYPERLINK", "1")

		require.True(t, cli.HyperlinksSupported(true))

		require.Equal(
			t,
			"\x1b]8;;"+url+"\x07\x1b[36;4mEdit token\x1b]8;;\x07\x1b[0m",
			cli.Hyperlink("Edit token", url, true),
		)
	})

	t.Run("when NO_COLOR is set", func(t *testing.T) {
		unsetTerminalEnv(t)
		t.Setenv("FORCE_HYPERLINK", "1")
		t.Setenv("NO_COLOR", "1")

		require.Equal(
			t,
			"\x1b]8;;"+url+"\x07Edit token\x1b]8;;\x07\x1b[0m",
			cli.Hyperlink("Edit token", url, true),
		)
	})
}
