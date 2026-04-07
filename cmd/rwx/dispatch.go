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

var (
	DispatchParams   []string
	DispatchOpen     bool
	DispatchDebug    bool
	DispatchWait     bool
	DispatchFailFast bool
	DispatchTitle    string
	DispatchRef      string

	dispatchCmd = &cobra.Command{
		GroupID: "api",
		Args:    cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireAccessToken()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			dispatchKey := args[0]

			params, err := ParseParams(DispatchParams)
			if err != nil {
				return errors.Wrap(err, "unable to parse params")
			}

			useJson := useJsonOutput()
			dispatchResult, err := service.InitiateDispatch(cli.InitiateDispatchConfig{
				DispatchKey: dispatchKey,
				Params:      params,
				Json:        useJson,
				Title:       DispatchTitle,
				Ref:         DispatchRef,
			})
			if err != nil {
				return err
			}

			stopDispatchSpinner := cli.Spin("Waiting for dispatch to start...", service.StdoutIsTTY, service.Stdout)

			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()

			var runs []cli.GetDispatchRun

			for range ticker.C {
				runs, err = service.GetDispatch(cli.GetDispatchConfig{DispatchId: dispatchResult.DispatchId})
				if errors.Is(err, errors.ErrRetry) {
					continue
				}

				stopDispatchSpinner()
				if err != nil {
					return err
				}

				break
			}

			if useJson && !DispatchWait {
				jsonOutput := struct {
					RunID  string
					RunURL string
				}{
					RunID:  runs[0].RunID,
					RunURL: runs[0].RunURL,
				}
				dispatchResultJson, err := json.Marshal(jsonOutput)
				if err != nil {
					return err
				}

				fmt.Println(string(dispatchResultJson))
			} else if !useJson {
				fmt.Printf("Run is watchable at %s\n", runs[0].RunURL)
			}

			if DispatchOpen {
				if err := open.Run(runs[0].RunURL); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to open browser.\n")
				}
			}

			if DispatchWait && !DispatchDebug {
				waitResult, err := service.GetRunStatus(cli.GetRunStatusConfig{
					RunID:    runs[0].RunID,
					Wait:     true,
					FailFast: DispatchFailFast,
					Json:     useJson,
				})
				if err != nil {
					return err
				}

				promptResult, promptErr := service.GetRunPrompt(runs[0].RunID)

				if useJson {
					jsonOutput := struct {
						RunID        string
						RunURL       string
						ResultStatus string
						Prompt       string `json:",omitempty"`
					}{
						RunID:        runs[0].RunID,
						RunURL:       runs[0].RunURL,
						ResultStatus: waitResult.ResultStatus,
					}
					if promptErr == nil {
						jsonOutput.Prompt = promptResult.Prompt
					}
					waitResultJson, err := json.Marshal(jsonOutput)
					if err != nil {
						return err
					}
					fmt.Println(string(waitResultJson))
				} else {
					fmt.Printf("Run result status: %s\n", waitResult.ResultStatus)

					if promptErr == nil {
						fmt.Printf("\n%s", promptResult.Prompt)
					}
				}

				if waitResult.ResultStatus != "succeeded" {
					return HandledError
				}
			}

			if DispatchDebug {
				fmt.Println()
				stopDebugSpinner := cli.Spin("Waiting for run to hit a breakpoint...", service.StdoutIsTTY, service.Stdout)

				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()

				for range ticker.C {
					stopDebugSpinner()
					err := service.DebugTask(cli.DebugTaskConfig{DebugKey: runs[0].RunID})
					if errors.Is(err, errors.ErrRetry) {
						stopDebugSpinner = cli.Spin("Waiting for run to hit a breakpoint...", service.StdoutIsTTY, service.Stdout)
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
		Short: "Launch a run from a pre-configured RWX workflow",
		Use:   "dispatch <dispatch-key> [flags]",
	}
)

func init() {
	dispatchCmd.Flags().StringArrayVar(&DispatchParams, "param", []string{}, "dispatch params for the run in form `key=value`, available in the `event.dispatch.params` context. Can be specified multiple times")
	dispatchCmd.Flags().StringVar(&DispatchRef, "ref", "", "the git ref to use for the run")
	dispatchCmd.Flags().BoolVar(&DispatchOpen, "open", false, "open the run in a browser")
	dispatchCmd.Flags().BoolVar(&DispatchDebug, "debug", false, "start a remote debugging session once a breakpoint is hit")
	dispatchCmd.Flags().BoolVar(&DispatchWait, "wait", false, "poll for the run to complete and report the result status")
	dispatchCmd.Flags().BoolVar(&DispatchFailFast, "fail-fast", false, "stop waiting when failures are available (only has an effect when used with --wait)")
	dispatchCmd.Flags().StringVar(&DispatchTitle, "title", "", "the title the UI will display for the run")
	dispatchCmd.Flags().SortFlags = false
}

// ParseParams converts a list of `key=value` pairs to a map.
func ParseParams(params []string) (map[string]string, error) {
	parsedParams := make(map[string]string)

	parse := func(p string) error {
		fields := strings.Split(p, "=")
		if len(fields) < 2 {
			return errors.Errorf("unable to parse %q", p)
		}

		parsedParams[fields[0]] = strings.Join(fields[1:], "=")
		return nil
	}

	for _, param := range params {
		if err := parse(param); err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return parsedParams, nil
}
