package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSandboxSyncCommandIsHidden(t *testing.T) {
	require.True(t, sandboxSyncCmd.Hidden)
	require.Equal(t, "sync", sandboxSyncCmd.Use)
	require.NotNil(t, sandboxSyncCmd.Flags().Lookup("id"))
	require.Nil(t, sandboxSyncCmd.Flags().Lookup("dir"))
	require.Nil(t, sandboxSyncCmd.Flags().Lookup("init"))
	require.NoError(t, sandboxSyncCmd.Args(sandboxSyncCmd, nil))
	require.Error(t, sandboxSyncCmd.Args(sandboxSyncCmd, []string{".rwx/sandbox.yml"}))
}

func TestSandboxBackgroundCommandIsHidden(t *testing.T) {
	require.True(t, sandboxBackgroundCmd.Hidden)
	require.Equal(t, "background -- <command>", sandboxBackgroundCmd.Use)
	require.NotNil(t, sandboxBackgroundCmd.PersistentFlags().Lookup("name"))
	require.NotNil(t, sandboxBackgroundCmd.Flags().Lookup("port"))
	require.NotNil(t, sandboxBackgroundCmd.Flags().Lookup("local-port"))
	require.Nil(t, sandboxBackgroundCmd.Flags().Lookup("dir"))
	require.Nil(t, sandboxBackgroundCmd.Flags().Lookup("init"))
	require.Equal(t, "restart", sandboxBackgroundRestartCmd.Use)
	require.True(t, sandboxBackgroundRestartCmd.Hidden)
	require.Equal(t, "stop", sandboxBackgroundStopCmd.Use)
	require.True(t, sandboxBackgroundStopCmd.Hidden)
	require.Equal(t, "logs", sandboxBackgroundLogsCmd.Use)
	require.True(t, sandboxBackgroundLogsCmd.Hidden)
	require.NotNil(t, sandboxBackgroundLogsCmd.Flags().Lookup("follow"))
}
