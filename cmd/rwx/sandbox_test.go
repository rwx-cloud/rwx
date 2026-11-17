package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	internalerrors "github.com/rwx-cloud/rwx/internal/errors"
	"github.com/rwx-cloud/rwx/internal/mocks"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestSandboxPushCommandIsVisible(t *testing.T) {
	require.False(t, sandboxPushCmd.Hidden)
	require.Equal(t, "push [config-file]", sandboxPushCmd.Use)
	require.NotNil(t, sandboxPushCmd.Flags().Lookup("id"))
	require.Nil(t, sandboxPushCmd.Flags().Lookup("dir"))
	require.Nil(t, sandboxPushCmd.Flags().Lookup("init"))
	require.NoError(t, sandboxPushCmd.Args(sandboxPushCmd, nil))
	require.NoError(t, sandboxPushCmd.Args(sandboxPushCmd, []string{".rwx/sandbox.yml"}))
	require.Error(t, sandboxPushCmd.Args(sandboxPushCmd, []string{"one.yml", "two.yml"}))
}

func TestSandboxBackgroundCommandsAreVisible(t *testing.T) {
	require.False(t, sandboxBackgroundCmd.Hidden)
	require.Equal(t, "background [config-file] -- <command>", sandboxBackgroundCmd.Use)
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
	require.Equal(t, "restart [config-file]", sandboxBackgroundRestartCmd.Use)
	require.False(t, sandboxBackgroundRestartCmd.Hidden)
	require.Equal(t, "stop [config-file]", sandboxBackgroundStopCmd.Use)
	require.False(t, sandboxBackgroundStopCmd.Hidden)
	require.Equal(t, "logs [config-file]", sandboxBackgroundLogsCmd.Use)
	require.False(t, sandboxBackgroundLogsCmd.Hidden)
	require.NotNil(t, sandboxBackgroundLogsCmd.Flags().Lookup("follow"))
	for _, cmd := range []*cobra.Command{sandboxBackgroundRestartCmd, sandboxBackgroundStopCmd, sandboxBackgroundLogsCmd} {
		require.NoError(t, cmd.Args(cmd, nil))
		require.NoError(t, cmd.Args(cmd, []string{".rwx/dev.yml"}))
		require.Error(t, cmd.Args(cmd, []string{"one.yml", "two.yml"}))
	}
}

func TestSandboxBackgroundHelpListsSubcommands(t *testing.T) {
	originalOutput := sandboxBackgroundCmd.OutOrStdout()
	t.Cleanup(func() { sandboxBackgroundCmd.SetOut(originalOutput) })
	var output bytes.Buffer
	sandboxBackgroundCmd.SetOut(&output)

	require.NoError(t, sandboxBackgroundCmd.Help())
	require.Contains(t, output.String(), "Available Commands:")
	require.Regexp(t, `(?m)^\s+restart\s+Sync changes and restart a sandbox background process$`, output.String())
	require.Regexp(t, `(?m)^\s+stop\s+Stop a sandbox background process$`, output.String())
	require.Regexp(t, `(?m)^\s+logs\s+Show logs for a sandbox background process$`, output.String())
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

func TestSandboxTunnelCommandIsVisible(t *testing.T) {
	require.False(t, sandboxTunnelCmd.Hidden)
	require.Equal(t, "tunnel [config-file]", sandboxTunnelCmd.Use)
	require.NotNil(t, sandboxTunnelCmd.Flags().Lookup("id"))
	require.NotNil(t, sandboxTunnelCmd.Flags().Lookup("key"))
	require.NotNil(t, sandboxTunnelCmd.Flags().Lookup("port"))
	require.NotNil(t, sandboxTunnelCmd.Flags().Lookup("local-port"))
	require.NotNil(t, sandboxTunnelCmd.Flags().Lookup("scheme"))
	require.Nil(t, sandboxTunnelCmd.Flags().Lookup("name"))
	require.NoError(t, sandboxTunnelCmd.Args(sandboxTunnelCmd, nil))
	require.NoError(t, sandboxTunnelCmd.Args(sandboxTunnelCmd, []string{".rwx/dev.yml"}))
	require.Error(t, sandboxTunnelCmd.Args(sandboxTunnelCmd, []string{"one.yml", "two.yml"}))
}

func TestSandboxCommandsSelectDefinition(t *testing.T) {
	originalService, originalFormat := service, Format
	originalRunID, originalName := sandboxRunID, sandboxBackgroundName
	originalTunnelKey, originalTunnelPort := sandboxTunnelKey, sandboxTunnelPort
	t.Cleanup(func() {
		service, Format = originalService, originalFormat
		sandboxRunID, sandboxBackgroundName = originalRunID, originalName
		sandboxTunnelKey, sandboxTunnelPort = originalTunnelKey, originalTunnelPort
	})
	Format = "json"
	sandboxBackgroundName, sandboxTunnelKey, sandboxTunnelPort = "web", "web", 3000

	for _, command := range []*cobra.Command{
		sandboxPushCmd, sandboxBackgroundCmd, sandboxBackgroundRestartCmd,
		sandboxBackgroundStopCmd, sandboxBackgroundLogsCmd, sandboxTunnelCmd,
	} {
		for _, scenario := range []string{"local", "explicit ID", "missing", "omitted definition", "ambiguous"} {
			t.Run(command.CommandPath()+"/"+scenario, func(t *testing.T) {
				t.Chdir(t.TempDir())
				require.NoError(t, os.Mkdir(".rwx", 0o755))
				configFile, err := filepath.Abs(".rwx/dev.yml")
				require.NoError(t, err)
				defaultFile, err := filepath.Abs(".rwx/sandbox.yml")
				require.NoError(t, err)
				storage, err := cli.LoadSandboxStorage()
				require.NoError(t, err)
				if scenario == "local" || scenario == "explicit ID" || scenario == "missing" || scenario == "ambiguous" {
					storage.SetSession("main", defaultFile, cli.SandboxSession{RunID: "run-default", ConfigFile: defaultFile})
				}
				if scenario == "local" || scenario == "explicit ID" || scenario == "omitted definition" || scenario == "ambiguous" {
					storage.SetSession("main", configFile, cli.SandboxSession{RunID: "run-custom", ConfigFile: configFile})
				}
				require.NoError(t, storage.Save())

				sandboxRunID = ""
				expectedRunID := "run-custom"
				expectedChecks := 2
				if scenario == "explicit ID" {
					sandboxRunID, expectedRunID, expectedChecks = "run-by-id", "run-by-id", 1
				} else if scenario == "omitted definition" {
					expectedChecks = 1
				} else if scenario == "ambiguous" {
					expectedRunID, expectedChecks = "run-default", 3
				}
				stopAfterSelection := errors.New("stop after sandbox selection")
				checks := 0
				mockAPI := &mocks.API{}
				mockAPI.MockGetSandboxConnectionInfo = func(runID, token string) (api.SandboxConnectionInfo, error) {
					checks++
					if scenario == "omitted definition" || scenario == "ambiguous" {
						require.Contains(t, []string{"run-default", "run-custom"}, runID)
						if scenario == "ambiguous" && checks == expectedChecks {
							require.Equal(t, expectedRunID, runID)
							return api.SandboxConnectionInfo{}, stopAfterSelection
						}
						return api.SandboxConnectionInfo{Sandboxable: true}, nil
					}
					require.Equal(t, expectedRunID, runID)
					if checks == expectedChecks {
						return api.SandboxConnectionInfo{}, stopAfterSelection
					}
					return api.SandboxConnectionInfo{Sandboxable: true}, nil
				}
				service = cli.Service{Config: cli.Config{
					APIClient: mockAPI,
					VCSClient: &mocks.VCS{MockGetBranch: "main", MockGetHead: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
					Stdout:    io.Discard, Stderr: io.Discard,
				}}

				args := []string{".rwx/dev.yml"}
				if scenario == "omitted definition" || scenario == "ambiguous" {
					args = nil
				}
				if command == sandboxBackgroundCmd {
					args = append(args, "--", "npm", "run", "dev")
				}
				cmd := &cobra.Command{Use: command.Use, Args: command.Args, RunE: command.RunE}
				cmd.SetContext(context.Background())
				require.NoError(t, cmd.ParseFlags(args))
				require.NoError(t, cmd.ValidateArgs(cmd.Flags().Args()))
				err = cmd.RunE(cmd, cmd.Flags().Args())
				if scenario == "missing" {
					require.ErrorContains(t, err, "No active sandbox found")
					require.ErrorContains(t, err, "rwx sandbox list")
					require.Zero(t, checks)
					return
				}
				if scenario == "omitted definition" {
					require.ErrorIs(t, err, internalerrors.ErrSandboxDefinitionRequired)
					require.ErrorContains(t, err, "No active sandbox is using the default definition")
					require.ErrorContains(t, err, "Specify a config file to select a non-default sandbox, or use --id")
					require.Equal(t, expectedChecks, checks)
					return
				}
				require.ErrorIs(t, err, stopAfterSelection)
				require.Equal(t, expectedChecks, checks)
			})
		}
	}
}

func TestSandboxBackgroundRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{
		{".rwx/dev.yml"},
		{".rwx/dev.yml", "--"},
		{"one.yml", "two.yml", "--", "npm", "run", "dev"},
	} {
		cmd := &cobra.Command{RunE: sandboxBackgroundCmd.RunE}
		require.NoError(t, cmd.ParseFlags(args))
		require.ErrorContains(t, cmd.RunE(cmd, cmd.Flags().Args()), "Usage: rwx sandbox background [config-file] -- <command>")
	}
}

func TestSandboxTransportSelection(t *testing.T) {
	originalTransport := sandboxTransport
	originalToken := AccessToken
	originalService := service
	originalBackend := accessTokenBackend

	t.Cleanup(func() {
		sandboxTransport = originalTransport
		AccessToken = originalToken
		service = originalService
		accessTokenBackend = originalBackend
	})

	t.Setenv("HOME", t.TempDir())
	AccessToken = "test-token"

	t.Run("selects WSS", func(t *testing.T) {
		sandboxTransport = "wss"

		require.NoError(t, rootCmd.PersistentPreRunE(sandboxExecCmd, nil))
		require.Equal(t, "wss", service.SandboxTransport)
	})

	t.Run("selects SSH", func(t *testing.T) {
		sandboxTransport = "ssh"

		require.NoError(t, rootCmd.PersistentPreRunE(sandboxExecCmd, nil))
		require.Equal(t, "ssh", service.SandboxTransport)
	})

	t.Run("invalid transport is rejected", func(t *testing.T) {
		sandboxTransport = "invalid"

		require.EqualError(t, rootCmd.PersistentPreRunE(sandboxExecCmd, nil), "--transport must be ssh or wss")
	})

	checkLifecycleTransport := func(t *testing.T, command *cobra.Command) {
		t.Helper()
		require.NoError(t, command.ParseFlags([]string{"--transport", "wss"}))
		flag := command.Flags().Lookup("transport")
		t.Cleanup(func() { flag.Changed = false })

		require.False(t, flag.Hidden)
		require.Equal(t, "ssh", flag.DefValue)

		require.NoError(t, rootCmd.PersistentPreRunE(command, nil))
		require.Equal(t, "wss", service.SandboxTransport)
	}

	t.Run("stop accepts WSS", func(t *testing.T) {
		checkLifecycleTransport(t, sandboxStopCmd)
	})

	t.Run("reset accepts WSS", func(t *testing.T) {
		checkLifecycleTransport(t, sandboxResetCmd)
	})

	t.Run("start accepts WSS", func(t *testing.T) {
		checkLifecycleTransport(t, sandboxStartCmd)
	})
}

func TestSandboxWSSProxyRejectsInsecureURL(t *testing.T) {
	command, _, err := rootCmd.Find([]string{"sandbox-wss-proxy"})
	require.NoError(t, err)
	require.Equal(t, "sandbox-wss-proxy", command.Name())

	require.ErrorContains(t, command.RunE(command, []string{"ws://insecure.invalid/ssh"}), "must be a wss:// URL")
}
