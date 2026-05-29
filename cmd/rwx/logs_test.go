package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogsOutputDirFlags(t *testing.T) {
	outputDirFlag := logsCmd.Flags().Lookup("output-dir")
	require.NotNil(t, outputDirFlag)
	require.False(t, outputDirFlag.Hidden)

	outputFileFlag := logsCmd.Flags().Lookup("output-file")
	require.NotNil(t, outputFileFlag)
	require.False(t, outputFileFlag.Hidden)

	require.Nil(t, logsCmd.Flags().Lookup("output-directory"))
}
