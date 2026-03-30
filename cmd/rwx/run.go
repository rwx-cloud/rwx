package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/errors"

	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
)

const flagInit = "init"

var (
	InitParameters []string
	RwxDirectory   string
	MintFilePath   string
	TargetedTasks  []string
	NoCache        bool
	Open           bool
	Debug          bool
	Wait           bool
	FailFast       bool
	Title          string

	runCmd = &cobra.Command{
		GroupID: "execution",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			for _, arg := range args {
				if strings.Contains(arg, "=") {
					initParam := strings.Split(arg, "=")[0]
					return fmt.Errorf(
						"You have specified a task target with an equals sign: \"%s\".\n"+
							"Are you trying to specify an init parameter \"%s\"?\n"+
							"You can define multiple init parameters by specifying --%s multiple times.\n"+
							"You may have meant to specify --%s \"%s\".",
						arg,
						initParam,
						flagInit,
						flagInit,
						arg,
					)
				}
			}

			fileFlag := cmd.Flags().Lookup("file")
			if (len(args) > 0 && fileFlag.Changed) || len(args) > 1 {
				return fmt.Errorf(
					"positional arguments are not supported for task targeting.\n" +
						"Use --target to specify task targets instead.\n" +
						"For example: rwx run <file> --target <task>",
				)
			}

			return requireAccessToken()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				MintFilePath = args[0]
			}

			initParams, err := ParseInitParameters(InitParameters)
			if err != nil {
				return errors.Wrap(err, "unable to parse init parameters")
			}

			useJson := useJsonOutput()

			runResult, err := service.InitiateRun(cli.InitiateRunConfig{
				InitParameters: initParams,
				Json:           useJson,
				RwxDirectory:   RwxDirectory,
				MintFilePath:   MintFilePath,
				NoCache:        NoCache,
				TargetedTasks:  TargetedTasks,
				Title:          Title,
				Patchable:      true,
			})
			if err != nil {
				return err
			}

			jsonOutput := struct {
				RunID            string
				RunURL           string
				TargetedTaskKeys []string
				DefinitionPath   string
				Message          string
				ResultStatus     string `json:",omitempty"`
			}{
				RunID:            runResult.RunID,
				RunURL:           runResult.RunURL,
				TargetedTaskKeys: runResult.TargetedTaskKeys,
				DefinitionPath:   runResult.DefinitionPath,
				Message:          strings.ReplaceAll(strings.ReplaceAll(runResult.Message, "\n\n", " "), "\n", " "),
			}

			if useJson && !Wait {
				runResultJson, err := json.Marshal(jsonOutput)
				if err != nil {
					return err
				}

				fmt.Println(string(runResultJson))
			} else if !useJson {
				fmt.Print(runResult.Message)
				if !Wait {
					fmt.Printf("\nUse `rwx results --wait %s` to wait for this run to complete.\n", runResult.RunID)
				}
			}

			if Open {
				if err := open.Run(runResult.RunURL); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to open browser.\n")
				}
			}

			if Wait && !Debug {
				waitResult, err := service.GetRunStatus(cli.GetRunStatusConfig{
					RunID:    runResult.RunID,
					Wait:     true,
					FailFast: FailFast,
					Json:     useJson,
				})
				if err != nil {
					return err
				}

				if useJson {
					jsonOutput.ResultStatus = waitResult.ResultStatus
					waitResultJson, err := json.Marshal(jsonOutput)
					if err != nil {
						return err
					}
					fmt.Println(string(waitResultJson))
				} else {
					fmt.Printf("Run result status: %s\n", waitResult.ResultStatus)

					promptResult, err := service.GetRunPrompt(runResult.RunID)
					if err == nil {
						fmt.Printf("\n%s", promptResult.Prompt)
					}
				}

				if waitResult.ResultStatus != "succeeded" {
					return HandledError
				}
			}

			if Debug {
				fmt.Println()
				stopSpinner := cli.Spin("Waiting for run to hit a breakpoint...", service.StdoutIsTTY, service.Stdout)

				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()

				for range ticker.C {
					stopSpinner()
					err := service.DebugTask(cli.DebugTaskConfig{DebugKey: runResult.RunID})
					if errors.Is(err, errors.ErrRetry) {
						stopSpinner = cli.Spin("Waiting for run to hit a breakpoint...", service.StdoutIsTTY, service.Stdout)
						continue
					}
					if errors.Is(err, errors.ErrGone) {
						fmt.Println("Run finished without encountering a breakpoint.")
						break
					}

					return err
				}
			}

			return nil

		},
		Short: "Launch a run from a local RWX definitions file",
		Long:  "Launch a run from a local RWX definitions file.\n\nThis is an alias for rwx runs create.",
		Use:   "run <file> [flags]",
	}
)

func init() {
	runCmd.Flags().BoolVar(&NoCache, "no-cache", false, "do not read or write to the cache")
	runCmd.Flags().StringArrayVar(&InitParameters, flagInit, []string{}, "initialization parameters for the run, available in the `init` context. Can be specified multiple times")
	runCmd.Flags().StringArrayVar(&TargetedTasks, "target", []string{}, "task to target for execution. Can be specified multiple times")
	runCmd.Flags().StringVarP(&MintFilePath, "file", "f", "", "an RWX config file to use for sourcing task definitions (required)")
	_ = runCmd.Flags().MarkHidden("file")
	addRwxDirFlag(runCmd)
	runCmd.Flags().BoolVar(&Open, "open", false, "open the run in a browser")
	runCmd.Flags().BoolVar(&Debug, "debug", false, "start a remote debugging session once a breakpoint is hit")
	runCmd.Flags().BoolVar(&Wait, "wait", false, "poll for the run to complete and report the result status")
	runCmd.Flags().BoolVar(&FailFast, "fail-fast", false, "stop waiting when failures are available (only has an effect when used with --wait)")
	runCmd.Flags().StringVar(&Title, "title", "", "the title the UI will display for the run")
}
