package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/manifoldco/promptui"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
)

var sandboxCmd = &cobra.Command{
	GroupID: "execution",
	Use:     "sandbox",
	Short:   "Run commands in persistent sandboxes",
}

var sandboxStartCmd = &cobra.Command{
	Use:   "start [config-file]",
	Short: "Start a sandbox without executing a command",
	Long: `Start a sandbox without executing a command.

FILE PATCHING
  When starting a new sandbox, RWX launches a run from the sandbox config and
  includes a git patch for local uncommitted changes when possible.

  Git LFS changes cannot be included in the initial sandbox patch and will
  produce an error. Commit and push those changes before starting the sandbox.
`,
	Args: cobra.MaximumNArgs(1),
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

  After the command completes, any changes made in the sandbox are
  automatically pulled back to the local working directory via git patch.
  This happens regardless of the command's exit code.

  If local changes include Git LFS files, syncing fails with an error before
  the command runs.

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

var sandboxPushCmd = &cobra.Command{
	Use:    "push",
	Short:  "Push local changes to a sandbox",
	Hidden: true,
	Args:   cobra.NoArgs,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return requireExperimentalSandboxAccess()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		useJson := useJsonOutput()
		result, err := service.SyncSandbox(cli.SyncSandboxConfig{
			RunID: sandboxRunID,
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

var sandboxBackgroundCmd = &cobra.Command{
	Use:    "background -- <command>",
	Short:  "Start or replace a sandbox background process",
	Hidden: true,
	Long: `Start or replace a named managed process in an existing sandbox.

The sandbox must already be running. By default, RWX uses the sandbox for the
current directory and git branch; use --id to target a specific sandbox run.

EXPERIMENTAL
  Background process commands are experimental. Set RWX_EXPERIMENTAL=true to
  use them.

PROCESS LIFECYCLE
  --name identifies the managed process. Names may contain letters, numbers,
  underscores, and hyphens. Starting an existing name replaces that process.
  The process continues running after this command returns and stops when the
  sandbox stops.

  Use the restart, stop, and logs subcommands to manage the process later.

FILE SYNCING
  Local staged, unstaged, and untracked changes are synced to the sandbox
  before the process starts. Changes made in the sandbox are not pulled back
  to the local working directory.

  Use "rwx sandbox push" to sync later local edits without restarting the
  process, or "rwx sandbox background restart" to sync and restart it.

PORT FORWARDING
  --port forwards a sandbox port to localhost and waits for it to become
  available. By default, RWX allocates a local port; use --local-port to
  request one. --scheme controls whether the printed URL uses http or https.`,
	Example: `  # Start a development server and forward its port
  RWX_EXPERIMENTAL=true rwx sandbox background --name web --port 3000 -- npm run dev

  # Start a worker without forwarding a port
  RWX_EXPERIMENTAL=true rwx sandbox background --name worker -- bin/worker

  # Target an existing sandbox by run ID
  RWX_EXPERIMENTAL=true rwx sandbox background --id run_123 --name web --port 3000 -- npm run dev`,
	Args: cobra.ArbitraryArgs,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return requireExperimentalSandboxAccess()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		dashIndex := cmd.ArgsLenAtDash()
		if dashIndex < 0 || dashIndex >= len(args) {
			return fmt.Errorf("No command specified. Usage: rwx sandbox background -- <command>")
		}
		if dashIndex != 0 {
			return fmt.Errorf("Unexpected arguments before '--'. Usage: rwx sandbox background -- <command>")
		}

		useJson := useJsonOutput()
		result, err := service.BackgroundSandbox(cli.BackgroundSandboxConfig{
			Command:    args[dashIndex:],
			Name:       sandboxBackgroundName,
			TargetPort: sandboxBackgroundPort,
			LocalPort:  sandboxBackgroundLocalPort,
			Scheme:     sandboxBackgroundScheme,
			RunID:      sandboxRunID,
			Json:       useJson,
		})
		if err != nil {
			return err
		}

		if useJson {
			output, err := json.Marshal(result)
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stdout, string(output))
		}
		return nil
	},
}

var sandboxBackgroundRestartCmd = &cobra.Command{
	Use:    "restart",
	Short:  "Sync changes and restart a sandbox background process",
	Hidden: true,
	Long: `Sync local changes and restart a named managed process.

The process must have been started with "rwx sandbox background". RWX reuses
its saved command and sandbox port. If the process had a forwarded port, RWX
reopens the local tunnel and prints its URL.

Local staged, unstaged, and untracked changes are synced before the restart.
Changes made in the sandbox are not pulled back to the local working
directory. Use --id to target a specific sandbox run.`,
	Example: `  RWX_EXPERIMENTAL=true rwx sandbox background restart --name web

  RWX_EXPERIMENTAL=true rwx sandbox background restart --id run_123 --name web`,
	Args: cobra.NoArgs,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return requireExperimentalSandboxAccess()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		useJson := useJsonOutput()
		result, err := service.RestartSandboxBackground(cli.SandboxBackgroundConfig{
			Name:  sandboxBackgroundName,
			RunID: sandboxRunID,
			Json:  useJson,
		})
		if err != nil {
			return err
		}
		if useJson {
			output, err := json.Marshal(result)
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stdout, string(output))
		}
		return nil
	},
}

var sandboxBackgroundStopCmd = &cobra.Command{
	Use:    "stop",
	Short:  "Stop a sandbox background process",
	Hidden: true,
	Long: `Stop a named managed process and close its local port-forwarding tunnel.

This command does not sync files. Use --id to target a specific sandbox run.`,
	Example: `  RWX_EXPERIMENTAL=true rwx sandbox background stop --name web

  RWX_EXPERIMENTAL=true rwx sandbox background stop --id run_123 --name web`,
	Args: cobra.NoArgs,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return requireExperimentalSandboxAccess()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		useJson := useJsonOutput()
		result, err := service.StopSandboxBackground(cli.SandboxBackgroundConfig{
			Name:  sandboxBackgroundName,
			RunID: sandboxRunID,
			Json:  useJson,
		})
		if err != nil {
			return err
		}
		if useJson {
			output, err := json.Marshal(result)
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stdout, string(output))
		}
		return nil
	},
}

var sandboxBackgroundLogsCmd = &cobra.Command{
	Use:    "logs",
	Short:  "Show logs for a sandbox background process",
	Hidden: true,
	Long: `Print logs for a named sandbox background process.

The name may identify a managed process started with "rwx sandbox background"
or a process declared under background-processes in the sandbox task. Without
--follow, RWX prints the available logs and exits. With --follow, RWX prints
existing logs, streams new output, and runs until interrupted.

This command does not sync files. Use --id to target a specific sandbox run.
With --json, RWX returns process metadata instead of printing log contents.`,
	Example: `  # Print the current logs
  RWX_EXPERIMENTAL=true rwx sandbox background logs --name web

  # Follow logs until interrupted
  RWX_EXPERIMENTAL=true rwx sandbox background logs --name web --follow

  # Read logs for a process declared in the sandbox configuration
  RWX_EXPERIMENTAL=true rwx sandbox background logs --name start-databases`,
	Args: cobra.NoArgs,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return requireExperimentalSandboxAccess()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		useJson := useJsonOutput()
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		result, err := service.LogsSandboxBackground(cli.SandboxBackgroundLogsConfig{
			Context: ctx,
			Name:    sandboxBackgroundName,
			RunID:   sandboxRunID,
			Json:    useJson,
			Follow:  sandboxBackgroundFollow,
		})
		if err != nil {
			return err
		}
		if useJson {
			output, err := json.Marshal(result)
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stdout, string(output))
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
	sandboxRunID               string
	sandboxStopAll             bool
	sandboxRwxDir              string
	sandboxOpen                bool
	sandboxWait                bool
	sandboxReset               bool
	sandboxBackgroundName      string
	sandboxBackgroundPort      int
	sandboxBackgroundLocalPort int
	sandboxBackgroundScheme    string
	sandboxBackgroundFollow    bool
	sandboxInitParams          []string
)

func requireExperimentalSandboxAccess() error {
	if os.Getenv("RWX_EXPERIMENTAL") != "true" {
		return fmt.Errorf("this command is experimental; set RWX_EXPERIMENTAL=true to use it")
	}
	return requireAccessToken()
}

func init() {
	sandboxCmd.AddCommand(sandboxInitCmd)
	sandboxCmd.AddCommand(sandboxStartCmd)
	sandboxCmd.AddCommand(sandboxExecCmd)
	sandboxCmd.AddCommand(sandboxPushCmd)
	sandboxBackgroundCmd.AddCommand(sandboxBackgroundRestartCmd)
	sandboxBackgroundCmd.AddCommand(sandboxBackgroundStopCmd)
	sandboxBackgroundCmd.AddCommand(sandboxBackgroundLogsCmd)
	sandboxCmd.AddCommand(sandboxBackgroundCmd)
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
	sandboxExecCmd.Flags().Bool("no-sync", false, "Deprecated; syncing always runs before execution")
	if err := sandboxExecCmd.Flags().MarkDeprecated("no-sync", "syncing always runs before execution"); err != nil {
		panic(err)
	}
	sandboxExecCmd.Flags().BoolVar(&sandboxReset, "reset", false, "Reset the sandbox before executing")
	sandboxExecCmd.Flags().StringArrayVar(&sandboxInitParams, "init", []string{}, "initialization parameters for the sandbox run, available in the `init` context. Can be specified multiple times")

	// push flags
	sandboxPushCmd.Flags().StringVar(&sandboxRunID, "id", "", "Use specific run ID")

	// background flags
	sandboxBackgroundCmd.PersistentFlags().StringVar(&sandboxRunID, "id", "", "Use specific run ID")
	sandboxBackgroundCmd.PersistentFlags().StringVar(&sandboxBackgroundName, "name", "", "Name of the managed sandbox process")
	sandboxBackgroundCmd.Flags().IntVar(&sandboxBackgroundPort, "port", 0, "Sandbox port to forward locally")
	sandboxBackgroundCmd.Flags().IntVar(&sandboxBackgroundLocalPort, "local-port", 0, "Local port to use (default: allocate or reuse one)")
	sandboxBackgroundCmd.Flags().StringVar(&sandboxBackgroundScheme, "scheme", "", "URL scheme for the forwarded port: http or https (default: http)")
	sandboxBackgroundLogsCmd.Flags().BoolVarP(&sandboxBackgroundFollow, "follow", "f", false, "Follow log output")
	if err := sandboxBackgroundCmd.MarkPersistentFlagRequired("name"); err != nil {
		panic(err)
	}

	// stop flags
	sandboxStopCmd.Flags().StringVar(&sandboxRunID, "id", "", "Stop specific sandbox by run ID")
	sandboxStopCmd.Flags().BoolVar(&sandboxStopAll, "all", false, "Stop all sandboxes")

	// reset flags
	sandboxResetCmd.Flags().StringVarP(&sandboxRwxDir, "dir", "d", "", "RWX directory")
	sandboxResetCmd.Flags().BoolVar(&sandboxOpen, "open", false, "Open the run in a browser")
	sandboxResetCmd.Flags().BoolVar(&sandboxWait, "wait", false, "Wait for sandbox to be ready")
	sandboxResetCmd.Flags().StringArrayVar(&sandboxInitParams, "init", []string{}, "initialization parameters for the sandbox run, available in the `init` context. Can be specified multiple times")

}
