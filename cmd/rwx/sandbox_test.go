package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSandboxExecNoSyncFlagSurface(t *testing.T) {
	flag := sandboxExecCmd.Flags().Lookup("no-sync")

	require.NotNil(t, flag)
	require.True(t, flag.Hidden)
	require.NotEmpty(t, flag.Deprecated)
	require.NotContains(t, strings.ToLower(sandboxExecCmd.Long), "no-sync")
}
