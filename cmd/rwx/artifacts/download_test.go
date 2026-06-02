package artifacts

import (
	"testing"

	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestDownloadOutputFlagSurface(t *testing.T) {
	InitDownload(func() error { return nil }, func() cli.Service { return cli.Service{} }, func() bool { return false })
	flags := DownloadCmd.Flags()

	require.NotNil(t, flags.Lookup("output"))

	outputDirFlag := flags.Lookup("output-dir")
	require.NotNil(t, outputDirFlag)
	require.True(t, outputDirFlag.Hidden)
	require.NotEmpty(t, outputDirFlag.Deprecated)

	outputFileFlag := flags.Lookup("output-file")
	require.NotNil(t, outputFileFlag)
	require.True(t, outputFileFlag.Hidden)
	require.NotEmpty(t, outputFileFlag.Deprecated)
}

func TestDownloadOutputFlagSet(t *testing.T) {
	t.Run("output is explicit when output-dir is set", func(t *testing.T) {
		cmd := newDownloadOutputFlagTestCommand(t)

		require.NoError(t, cmd.Flags().Set("output-dir", "artifacts"))

		outputSet, outputFileSet, err := downloadOutputFlagSet(cmd)
		require.NoError(t, err)
		require.True(t, outputSet)
		require.False(t, outputFileSet)
	})

	t.Run("output is explicit when output-file is set", func(t *testing.T) {
		cmd := newDownloadOutputFlagTestCommand(t)

		require.NoError(t, cmd.Flags().Set("output-file", "artifact.tar"))

		outputSet, outputFileSet, err := downloadOutputFlagSet(cmd)
		require.NoError(t, err)
		require.True(t, outputSet)
		require.True(t, outputFileSet)
	})

	t.Run("output flags are mutually exclusive", func(t *testing.T) {
		cmd := newDownloadOutputFlagTestCommand(t)

		require.NoError(t, cmd.Flags().Set("output", "artifacts"))
		require.NoError(t, cmd.Flags().Set("output-file", "artifact.tar"))

		_, _, err := downloadOutputFlagSet(cmd)
		require.EqualError(t, err, "--output, --output-dir, and --output-file cannot be used together")
	})
}

func newDownloadOutputFlagTestCommand(t *testing.T) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{}
	cmd.Flags().String("output", "", "")
	cmd.Flags().String("output-dir", "", "")
	cmd.Flags().String("output-file", "", "")
	return cmd
}
