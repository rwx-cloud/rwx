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
	nameFlag := sandboxBackgroundCmd.PersistentFlags().Lookup("name")
	keyFlag := sandboxBackgroundCmd.PersistentFlags().Lookup("key")
	require.NotNil(t, nameFlag)
	require.True(t, nameFlag.Hidden)
	require.Equal(t, "use --key instead", nameFlag.Deprecated)
	require.NotNil(t, keyFlag)
	require.False(t, keyFlag.Hidden)
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

func TestSandboxBackgroundNameAndKeyFlags(t *testing.T) {
	nameFlag := sandboxBackgroundCmd.PersistentFlags().Lookup("name")
	keyFlag := sandboxBackgroundCmd.PersistentFlags().Lookup("key")
	originalNameChanged := nameFlag.Changed
	originalKeyChanged := keyFlag.Changed
	originalName := sandboxBackgroundName
	t.Cleanup(func() {
		nameFlag.Changed = originalNameChanged
		keyFlag.Changed = originalKeyChanged
		require.NoError(t, keyFlag.Value.Set(originalName))
	})

	commands := []*cobra.Command{
		sandboxBackgroundCmd,
		sandboxBackgroundRestartCmd,
		sandboxBackgroundStopCmd,
		sandboxBackgroundLogsCmd,
	}

	nameFlag.Changed = false
	keyFlag.Changed = false
	for _, command := range commands {
		require.Error(t, command.ValidateFlagGroups(), "%s should require --key or its compatibility alias", command.CommandPath())
	}

	require.NoError(t, keyFlag.Value.Set("worker"))
	require.Equal(t, "worker", sandboxBackgroundName)
	keyFlag.Changed = true
	for _, command := range commands {
		require.NoError(t, command.ValidateFlagGroups(), "%s should accept --key", command.CommandPath())
	}

	nameFlag.Changed = true
	for _, command := range commands {
		require.Error(t, command.ValidateFlagGroups(), "%s should reject --name with --key", command.CommandPath())
	}
}

func TestSandboxTunnelCommandIsHidden(t *testing.T) {
	require.True(t, sandboxTunnelCmd.Hidden)
	require.Equal(t, "tunnel", sandboxTunnelCmd.Use)
	require.NotNil(t, sandboxTunnelCmd.Flags().Lookup("id"))
	require.NotNil(t, sandboxTunnelCmd.Flags().Lookup("key"))
	require.NotNil(t, sandboxTunnelCmd.Flags().Lookup("port"))
	require.NotNil(t, sandboxTunnelCmd.Flags().Lookup("local-port"))
	require.NotNil(t, sandboxTunnelCmd.Flags().Lookup("scheme"))
	require.Nil(t, sandboxTunnelCmd.Flags().Lookup("name"))
	require.NoError(t, sandboxTunnelCmd.Args(sandboxTunnelCmd, nil))
	require.Error(t, sandboxTunnelCmd.Args(sandboxTunnelCmd, []string{"web"}))
}

func TestExperimentalSandboxCommandsRequireOptIn(t *testing.T) {
	commands := []*cobra.Command{
		sandboxPushCmd,
		sandboxBackgroundCmd,
		sandboxBackgroundRestartCmd,
		sandboxBackgroundStopCmd,
		sandboxBackgroundLogsCmd,
		sandboxTunnelCmd,
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
