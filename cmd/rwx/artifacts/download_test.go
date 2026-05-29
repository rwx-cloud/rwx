package artifacts

import (
	"testing"

	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestDownloadOutputDirFlags(t *testing.T) {
	InitDownload(func() error { return nil }, func() cli.Service { return cli.Service{} }, func() bool { return false })

	outputDirFlag := DownloadCmd.Flags().Lookup("output-dir")
	require.NotNil(t, outputDirFlag)
	require.False(t, outputDirFlag.Hidden)

	require.Nil(t, DownloadCmd.Flags().Lookup("output-directory"))
	require.Nil(t, DownloadCmd.Flags().Lookup("output-file"))
}
