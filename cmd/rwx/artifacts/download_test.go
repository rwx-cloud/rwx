package artifacts

import (
	"testing"

	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestDownloadOutputFlagSurface(t *testing.T) {
	InitDownload(func() error { return nil }, func() cli.Service { return cli.Service{} }, func() bool { return false })
	flags := DownloadCmd.Flags()

	require.NotNil(t, flags.Lookup("output"))
	require.Nil(t, flags.Lookup("output-dir"))
	require.Nil(t, flags.Lookup("output-file"))
}
