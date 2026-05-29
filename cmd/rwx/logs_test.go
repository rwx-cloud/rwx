package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogsOutputDirectoryFlags(t *testing.T) {
	outputDirectoryFlag := logsCmd.Flags().Lookup("output-directory")
	require.NotNil(t, outputDirectoryFlag)
	require.False(t, outputDirectoryFlag.Hidden)

	outputDirAlias := logsCmd.Flags().Lookup("output-dir")
	require.NotNil(t, outputDirAlias)
	require.True(t, outputDirAlias.Hidden)

	require.Nil(t, logsCmd.Flags().Lookup("output-file"))
}
