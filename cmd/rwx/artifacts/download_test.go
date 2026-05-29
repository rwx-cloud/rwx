package artifacts

import (
	"testing"

	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestDownloadOutputDirectoryFlags(t *testing.T) {
	InitDownload(func() error { return nil }, func() cli.Service { return cli.Service{} }, func() bool { return false })

	outputDirectoryFlag := DownloadCmd.Flags().Lookup("output-directory")
	require.NotNil(t, outputDirectoryFlag)
	require.False(t, outputDirectoryFlag.Hidden)

	outputDirAlias := DownloadCmd.Flags().Lookup("output-dir")
	require.NotNil(t, outputDirAlias)
	require.True(t, outputDirAlias.Hidden)

	require.Nil(t, DownloadCmd.Flags().Lookup("output-file"))
}
