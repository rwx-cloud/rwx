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
	"github.com/rwx-cloud/rwx/internal/mocks"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestSandboxPushCommandIsHidden(t *testing.T) {
	require.True(t, sandboxPushCmd.Hidden)
	require.Equal(t, "push [config-file]", sandboxPushCmd.Use)
	require.NotNil(t, sandboxPushCmd.Flags().Lookup("id"))
	require.Nil(t, sandboxPushCmd.Flags().Lookup("dir"))
	require.Nil(t, sandboxPushCmd.Flags().Lookup("init"))
	require.NoError(t, sandboxPushCmd.Args(sandboxPushCmd, nil))
	require.NoError(t, sandboxPushCmd.Args(sandboxPushCmd, []string{".rwx/sandbox.yml"}))
	require.Error(t, sandboxPushCmd.Args(sandboxPushCmd, []string{"one.yml", "two.yml"}))
}

func TestSandboxBackgroundCommandIsHidden(t *testing.T) {
	require.True(t, sandboxBackgroundCmd.Hidden)
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

func TestSandboxTunnelCommandIsHidden(t *testing.T) {
	require.True(t, sandboxTunnelCmd.Hidden)
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
		for _, scenario := range []string{"local", "remote", "explicit ID", "missing", "omitted definition", "ambiguous"} {
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
				}
				stopAfterSelection := errors.New("stop after sandbox selection")
				checks := 0
				mockAPI := &mocks.API{}
				mockAPI.MockGetSandboxConnectionInfo = func(runID, token string) (api.SandboxConnectionInfo, error) {
					checks++
					if scenario == "ambiguous" {
						require.Contains(t, []string{"run-default", "run-custom"}, runID)
						return api.SandboxConnectionInfo{Sandboxable: true}, nil
					}
					require.Equal(t, expectedRunID, runID)
					if checks == expectedChecks {
						return api.SandboxConnectionInfo{}, stopAfterSelection
					}
					return api.SandboxConnectionInfo{Sandboxable: true}, nil
				}
				mockAPI.MockListSandboxRuns = func() (*api.ListSandboxRunsResult, error) {
					require.Contains(t, []string{"remote", "missing"}, scenario)
					defaultState := cli.EncodeCliState("main", defaultFile)
					runs := []api.RunSummary{{ID: "run-default", CliState: &defaultState}}
					if scenario == "remote" {
						customState := cli.EncodeCliState("main", configFile)
						runs = append(runs, api.RunSummary{ID: "run-custom", CliState: &customState})
					}
					return &api.ListSandboxRunsResult{Runs: runs}, nil
				}
				mockAPI.MockCreateSandboxToken = func(cfg api.CreateSandboxTokenConfig) (*api.CreateSandboxTokenResult, error) {
					require.Equal(t, "run-custom", cfg.RunID)
					return &api.CreateSandboxTokenResult{Token: "recovered-token"}, nil
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
					require.Zero(t, checks)
					return
				}
				if scenario == "ambiguous" {
					require.ErrorContains(t, err, "Multiple active sandboxes found")
					require.ErrorContains(t, err, "Specify a config file to select one, or use --id")
					require.Equal(t, 2, checks)
					return
				}
				require.ErrorIs(t, err, stopAfterSelection)
				require.Equal(t, expectedChecks, checks)
				if scenario == "remote" {
					storage, err := cli.LoadSandboxStorage()
					require.NoError(t, err)
					session, found := storage.GetSession("main", configFile)
					require.True(t, found)
					require.Equal(t, "run-custom", session.RunID)
					require.Equal(t, "recovered-token", session.ScopedToken)
				}
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
