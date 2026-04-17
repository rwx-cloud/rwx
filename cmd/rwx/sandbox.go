package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/manifoldco/promptui"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
)

var sandboxCmd = &cobra.Command{
	GroupID: "execution",
	Use:     "sandbox",
	Short:   "Run commands in persistent sandboxes",
	Hidden:  true,
}

var sandboxStartCmd = &cobra.Command{
	Use:   "start [config-file]",
	Short: "Start a sandbox without executing a command",
	Args:  cobra.MaximumNArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return requireAccessToken()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		configFile := cli.FindDefaultSandboxConfigFile()
		if len(args) > 0 {
			configFile = args[0]
		}
		configFile = cli.AbsConfigFile(configFile)

		useJson := useJsonOutput()

		initParams, err := ParseInitParameters(sandboxInitParams)
		if err != nil {
			return fmt.Errorf("unable to parse init parameters: %w", err)
		}

		// Check for existing active sandbox (skip if --id is provided)
		if sandboxRunID == "" {
			existing, err := service.CheckExistingSandbox(configFile)
			if err != nil {
				return err
			}

			if existing.Exists && existing.Active {
				// Prompt user for what to do
				fmt.Fprintf(os.Stdout, "An active sandbox already exists for this directory and branch:\n")
				fmt.Fprintf(os.Stdout, "  Run ID: %s\n", existing.RunID)
				fmt.Fprintf(os.Stdout, "  URL: %s\n\n", existing.RunURL)

				prompt := promptui.Select{
					Label: "What would you like to do",
					Items: []string{"Continue with existing sandbox", "Stop and start a new sandbox"},
				}

				idx, _, err := prompt.Run()
				if err != nil {
					return err
				}

				if idx == 0 {
					// Continue with existing
					if sandboxOpen && existing.RunURL != "" {
						if openErr := open.Run(existing.RunURL); openErr != nil {
							fmt.Fprintf(os.Stderr, "Failed to open browser.\n")
						}
					}

					if useJson {
						result := cli.StartSandboxResult{
							RunID:      existing.RunID,
							RunURL:     existing.RunURL,
							ConfigFile: existing.ConfigFile,
						}
						jsonOutput, err := json.Marshal(result)
						if err != nil {
							return err
						}
						fmt.Println(string(jsonOutput))
					} else {
						fmt.Fprintf(os.Stdout, "Using existing sandbox: %s\n", existing.RunID)
					}
					return nil
				}

				// User chose to reset - stop existing and continue to start new
				_, err = service.StopSandbox(cli.StopSandboxConfig{
					RunID: existing.RunID,
					Json:  useJson,
				})
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: Failed to stop existing sandbox: %v\n", err)
				}
			}
		}

		result, err := service.StartSandbox(cli.StartSandboxConfig{
			ConfigFile:     configFile,
			RunID:          sandboxRunID,
			RwxDirectory:   sandboxRwxDir,
			Json:           useJson,
			Wait:           sandboxWait,
			InitParameters: initParams,
		})

		// Open browser if we have a URL, even if there was an error
		if sandboxOpen && result != nil && result.RunURL != "" {
			if openErr := open.Run(result.RunURL); openErr != nil {
				fmt.Fprintf(os.Stderr, "Failed to open browser.\n")
			}
		}

		if err != nil {
			return err
		}

		if useJson {
			jsonOutput, err := json.Marshal(result)
			if err != nil {
				return err
			}
			fmt.Println(string(jsonOutput))
		}

		return nil
	},
}

var sandboxExecCmd = &cobra.Command{
	Use:   "exec [config-file] -- <command>",
	Short: "Execute a command in a sandbox",
	Long: `Execute a command in a persistent cloud sandbox environment.

OVERVIEW
  Sandboxes are isolated, reproducible environments running in RWX cloud
  infrastructure. They persist between commands, allowing you to run multiple
  commands against the same environment without rebuilding each time.

FILE SYNCING
  Before each command, local uncommitted changes are automatically synced to
  the sandbox via git patch. This includes staged changes, unstaged changes,
  and untracked files.
  Use --no-sync to skip this step if you want to run against the sandbox's
  original state.

  After the command completes, any changes made in the sandbox are
  automatically pulled back to the local working directory via git patch.
  This happens regardless of the command's exit code.

  Note: Git LFS files cannot be synced and will generate a warning.

CONFIG FILE
  The sandbox configuration (default: .rwx/sandbox.yml) defines:
    - Base image and dependencies
    - Git repository to clone
    - Any setup tasks that run before the sandbox becomes available

  The config must include a task with "run: rwx-sandbox" which defines the
  sandbox entry point, and must be dependent on a task that uses git/clone.
`,
	Args: cobra.ArbitraryArgs,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return requireAccessToken()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get command args after --
		dashIndex := cmd.ArgsLenAtDash()
		var command []string
		var configFile string

		if dashIndex < 0 {
			// No -- found, error
			return fmt.Errorf("No command specified. Usage: rwx sandbox exec [config-file] -- <command>")
		}

		// Args before -- are config file (optional)
		if dashIndex > 0 {
			configFile = cli.AbsConfigFile(args[0])
		}

		// Args after -- are the command
		if dashIndex < len(args) {
			command = args[dashIndex:]
		}

		if len(command) == 0 {
			return fmt.Errorf("No command specified. Usage: rwx sandbox exec [config-file] -- <command>")
		}

		useJson := useJsonOutput()

		initParams, err := ParseInitParameters(sandboxInitParams)
		if err != nil {
			return fmt.Errorf("unable to parse init parameters: %w", err)
		}

		result, err := service.ExecSandbox(cli.ExecSandboxConfig{
			ConfigFile:     configFile,
			Command:        command,
			RunID:          sandboxRunID,
			RwxDirectory:   sandboxRwxDir,
			Json:           useJson,
			Sync:           !sandboxNoSync,
			InitParameters: initParams,
			Reset:          sandboxReset,
		})
		if err != nil {
			return err
		}

		if useJson {
			jsonOutput, err := json.Marshal(result)
			if err != nil {
				return err
			}
			fmt.Println(string(jsonOutput))
		}

		if sandboxOpen {
			if err := open.Run(result.RunURL); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to open browser.\n")
			}
		}

		if result.ExitCode != 0 {
			return &cli.ExitCodeError{Code: result.ExitCode}
		}
		return nil
	},
}

var sandboxListCmd = &cobra.Command{
	Use:   "list",
	Short: "List sandbox sessions with status",
	Args:  cobra.NoArgs,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return requireAccessToken()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		useJson := useJsonOutput()
		result, err := service.ListSandboxes(cli.ListSandboxesConfig{
			Json: useJson,
		})
		if err != nil {
			return err
		}

		if useJson {
			jsonOutput, err := json.Marshal(result)
			if err != nil {
				return err
			}
			fmt.Println(string(jsonOutput))
		}

		return nil
	},
}

var sandboxStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a sandbox session",
	Args:  cobra.NoArgs,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return requireAccessToken()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		useJson := useJsonOutput()
		result, err := service.StopSandbox(cli.StopSandboxConfig{
			RunID: sandboxRunID,
			All:   sandboxStopAll,
			Json:  useJson,
		})
		if err != nil {
			return err
		}

		if useJson {
			jsonOutput, err := json.Marshal(result)
			if err != nil {
				return err
			}
			fmt.Println(string(jsonOutput))
		}

		return nil
	},
}

var sandboxInitCmd = &cobra.Command{
	Use:   "init [output-file]",
	Short: "Initialize a sandbox configuration file",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		outputFile := ".rwx/sandbox.yml"
		if len(args) > 0 {
			outputFile = args[0]
		}

		if _, err := os.Stat(outputFile); err == nil {
			fmt.Fprintf(os.Stderr, "File already exists: %s\n", outputFile)
			return nil
		}

		useJson := useJsonOutput()
		result, err := service.GetSandboxInitTemplate(cli.GetSandboxInitTemplateConfig{
			Json: useJson,
		})
		if err != nil {
			return err
		}

		// Create parent directory if it doesn't exist
		dir := filepath.Dir(outputFile)
		if dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
		}

		if err := os.WriteFile(outputFile, []byte(result.Template), 0644); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}

		fmt.Fprintf(os.Stdout, "Created sandbox configuration: %s\n", outputFile)
		return nil
	},
}

var sandboxResetCmd = &cobra.Command{
	Use:   "reset [config-file]",
	Short: "Stop and restart a sandbox",
	Args:  cobra.MaximumNArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return requireAccessToken()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		configFile := cli.FindDefaultSandboxConfigFile()
		if len(args) > 0 {
			configFile = args[0]
		}
		configFile = cli.AbsConfigFile(configFile)

		useJson := useJsonOutput()

		initParams, err := ParseInitParameters(sandboxInitParams)
		if err != nil {
			return fmt.Errorf("unable to parse init parameters: %w", err)
		}

		result, err := service.ResetSandbox(cli.ResetSandboxConfig{
			ConfigFile:     configFile,
			RwxDirectory:   sandboxRwxDir,
			Json:           useJson,
			Wait:           sandboxWait,
			InitParameters: initParams,
		})
		if err != nil {
			return err
		}

		if useJson {
			jsonOutput, err := json.Marshal(result)
			if err != nil {
				return err
			}
			fmt.Println(string(jsonOutput))
		}

		if sandboxOpen {
			if err := open.Run(result.RunURL); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to open browser.\n")
			}
		}

		return nil
	},
}

var (
	sandboxRunID      string
	sandboxStopAll    bool
	sandboxRwxDir     string
	sandboxOpen       bool
	sandboxWait       bool
	sandboxNoSync     bool
	sandboxReset      bool
	sandboxInitParams []string
)

func init() {
	sandboxCmd.AddCommand(sandboxInitCmd)
	sandboxCmd.AddCommand(sandboxStartCmd)
	sandboxCmd.AddCommand(sandboxExecCmd)
	sandboxCmd.AddCommand(sandboxListCmd)
	sandboxCmd.AddCommand(sandboxStopCmd)
	sandboxCmd.AddCommand(sandboxResetCmd)

	// start flags
	sandboxStartCmd.Flags().StringVarP(&sandboxRwxDir, "dir", "d", "", "RWX directory")
	sandboxStartCmd.Flags().StringVar(&sandboxRunID, "id", "", "Use specific run ID")
	sandboxStartCmd.Flags().BoolVar(&sandboxOpen, "open", false, "Open the run in a browser")
	sandboxStartCmd.Flags().BoolVar(&sandboxWait, "wait", false, "Wait for sandbox to be ready")
	sandboxStartCmd.Flags().StringArrayVar(&sandboxInitParams, "init", []string{}, "initialization parameters for the sandbox run, available in the `init` context. Can be specified multiple times")

	// exec flags
	sandboxExecCmd.Flags().StringVarP(&sandboxRwxDir, "dir", "d", "", "RWX directory")
	sandboxExecCmd.Flags().StringVar(&sandboxRunID, "id", "", "Use specific run ID")
	sandboxExecCmd.Flags().BoolVar(&sandboxOpen, "open", false, "Open the run in a browser")
	sandboxExecCmd.Flags().BoolVar(&sandboxNoSync, "no-sync", false, "Skip syncing local changes before execution")
	sandboxExecCmd.Flags().BoolVar(&sandboxReset, "reset", false, "Reset the sandbox before executing")
	sandboxExecCmd.Flags().StringArrayVar(&sandboxInitParams, "init", []string{}, "initialization parameters for the sandbox run, available in the `init` context. Can be specified multiple times")

	// stop flags
	sandboxStopCmd.Flags().StringVar(&sandboxRunID, "id", "", "Stop specific sandbox by run ID")
	sandboxStopCmd.Flags().BoolVar(&sandboxStopAll, "all", false, "Stop all sandboxes")

	// reset flags
	sandboxResetCmd.Flags().StringVarP(&sandboxRwxDir, "dir", "d", "", "RWX directory")
	sandboxResetCmd.Flags().BoolVar(&sandboxOpen, "open", false, "Open the run in a browser")
	sandboxResetCmd.Flags().BoolVar(&sandboxWait, "wait", false, "Wait for sandbox to be ready")
	sandboxResetCmd.Flags().StringArrayVar(&sandboxInitParams, "init", []string{}, "initialization parameters for the sandbox run, available in the `init` context. Can be specified multiple times")

}
