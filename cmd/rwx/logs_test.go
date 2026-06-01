package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogsOutputFlagSurface(t *testing.T) {
	flags := logsCmd.Flags()

	require.NotNil(t, flags.Lookup("output"))
	require.Nil(t, flags.Lookup("output-dir"))
	require.Nil(t, flags.Lookup("output-file"))

	autoExtractFlag := flags.Lookup("auto-extract")
	require.NotNil(t, autoExtractFlag)
	require.True(t, autoExtractFlag.Hidden)
}
