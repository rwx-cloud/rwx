package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSandboxPushCommandIsVisible(t *testing.T) {
	require.False(t, sandboxPushCmd.Hidden)
	require.Equal(t, "push", sandboxPushCmd.Use)
	require.NotNil(t, sandboxPushCmd.Flags().Lookup("id"))
	require.Nil(t, sandboxPushCmd.Flags().Lookup("dir"))
	require.Nil(t, sandboxPushCmd.Flags().Lookup("init"))
	require.NoError(t, sandboxPushCmd.Args(sandboxPushCmd, nil))
	require.Error(t, sandboxPushCmd.Args(sandboxPushCmd, []string{".rwx/sandbox.yml"}))
}

func TestSandboxBackgroundCommandsAreVisible(t *testing.T) {
	require.False(t, sandboxBackgroundCmd.Hidden)
	require.Equal(t, "background -- <command>", sandboxBackgroundCmd.Use)
	require.NotNil(t, sandboxBackgroundCmd.PersistentFlags().Lookup("name"))
	require.NotNil(t, sandboxBackgroundCmd.Flags().Lookup("port"))
	require.NotNil(t, sandboxBackgroundCmd.Flags().Lookup("local-port"))
	require.Nil(t, sandboxBackgroundCmd.Flags().Lookup("dir"))
	require.Nil(t, sandboxBackgroundCmd.Flags().Lookup("init"))
	require.Equal(t, "restart", sandboxBackgroundRestartCmd.Use)
	require.False(t, sandboxBackgroundRestartCmd.Hidden)
	require.Equal(t, "stop", sandboxBackgroundStopCmd.Use)
	require.False(t, sandboxBackgroundStopCmd.Hidden)
	require.Equal(t, "logs", sandboxBackgroundLogsCmd.Use)
	require.False(t, sandboxBackgroundLogsCmd.Hidden)
	require.NotNil(t, sandboxBackgroundLogsCmd.Flags().Lookup("follow"))
}
