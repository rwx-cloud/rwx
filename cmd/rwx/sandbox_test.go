package main

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestSandboxPushCommandIsHidden(t *testing.T) {
	require.True(t, sandboxPushCmd.Hidden)
	require.Equal(t, "push", sandboxPushCmd.Use)
	require.NotNil(t, sandboxPushCmd.Flags().Lookup("id"))
	require.Nil(t, sandboxPushCmd.Flags().Lookup("dir"))
	require.Nil(t, sandboxPushCmd.Flags().Lookup("init"))
	require.NoError(t, sandboxPushCmd.Args(sandboxPushCmd, nil))
	require.Error(t, sandboxPushCmd.Args(sandboxPushCmd, []string{".rwx/sandbox.yml"}))
}

func TestSandboxBackgroundCommandIsHidden(t *testing.T) {
	require.True(t, sandboxBackgroundCmd.Hidden)
	require.Equal(t, "background -- <command>", sandboxBackgroundCmd.Use)
	require.Contains(t, sandboxBackgroundCmd.Long, "PROCESS LIFECYCLE")
	require.Contains(t, sandboxBackgroundCmd.Long, "FILE SYNCING")
	require.Contains(t, sandboxBackgroundCmd.Long, "PORT FORWARDING")
	require.NotEmpty(t, sandboxBackgroundCmd.Example)
	require.NotNil(t, sandboxBackgroundCmd.PersistentFlags().Lookup("name"))
	require.NotNil(t, sandboxBackgroundCmd.Flags().Lookup("port"))
	require.NotNil(t, sandboxBackgroundCmd.Flags().Lookup("local-port"))
	require.Nil(t, sandboxBackgroundCmd.Flags().Lookup("dir"))
	require.Nil(t, sandboxBackgroundCmd.Flags().Lookup("init"))
	require.Equal(t, "restart", sandboxBackgroundRestartCmd.Use)
	require.True(t, sandboxBackgroundRestartCmd.Hidden)
	require.Contains(t, sandboxBackgroundRestartCmd.Long, "saved command")
	require.NotEmpty(t, sandboxBackgroundRestartCmd.Example)
	require.Equal(t, "stop", sandboxBackgroundStopCmd.Use)
	require.True(t, sandboxBackgroundStopCmd.Hidden)
	require.Contains(t, sandboxBackgroundStopCmd.Long, "does not sync files")
	require.NotEmpty(t, sandboxBackgroundStopCmd.Example)
	require.Equal(t, "logs", sandboxBackgroundLogsCmd.Use)
	require.True(t, sandboxBackgroundLogsCmd.Hidden)
	require.Contains(t, sandboxBackgroundLogsCmd.Long, "background-processes")
	require.Contains(t, sandboxBackgroundLogsCmd.Long, "--follow")
	require.NotEmpty(t, sandboxBackgroundLogsCmd.Example)
	require.NotNil(t, sandboxBackgroundLogsCmd.Flags().Lookup("follow"))
}

func TestExperimentalSandboxCommandsRequireOptIn(t *testing.T) {
	commands := []*cobra.Command{
		sandboxPushCmd,
		sandboxBackgroundCmd,
		sandboxBackgroundRestartCmd,
		sandboxBackgroundStopCmd,
		sandboxBackgroundLogsCmd,
	}

	for _, value := range []string{"", "false", "TRUE", "1"} {
		t.Run("RWX_EXPERIMENTAL="+value, func(t *testing.T) {
			t.Setenv("RWX_EXPERIMENTAL", value)
			for _, command := range commands {
				require.EqualError(t, command.PreRunE(command, nil), "this command is experimental; set RWX_EXPERIMENTAL=true to use it")
			}
		})
	}

	t.Run("RWX_EXPERIMENTAL=true", func(t *testing.T) {
		t.Setenv("RWX_EXPERIMENTAL", "true")
		originalAccessToken := AccessToken
		AccessToken = "test-token"
		t.Cleanup(func() { AccessToken = originalAccessToken })
		for _, command := range commands {
			require.NoError(t, command.PreRunE(command, nil))
		}
	})
}
