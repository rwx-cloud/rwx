package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/mocks"
	"github.com/stretchr/testify/require"
)

func TestDispatchOutput(t *testing.T) {
	cases := []struct {
		name   string
		flag   string
		value  string
		wait   bool
		tty    bool
		failed bool
	}{
		{name: "json", flag: "json", value: "true"},
		{name: "format json", flag: "format", value: "json"},
		{name: "json wait", flag: "json", value: "true", wait: true},
		{name: "format json wait", flag: "format", value: "json", wait: true},
		{name: "json wait failed", flag: "json", value: "true", wait: true, failed: true},
		{name: "json tty", flag: "json", value: "true", tty: true},
		{name: "text", flag: "format", value: "text"},
		{name: "text wait", flag: "format", value: "text", wait: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			originalService, originalStdout := service, os.Stdout
			originalFormat, originalWait := Format, DispatchWait
			t.Cleanup(func() {
				service, os.Stdout = originalService, originalStdout
				Format, DispatchWait = originalFormat, originalWait
			})

			stdoutPath := filepath.Join(t.TempDir(), "stdout")
			stdout, err := os.Create(stdoutPath)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, stdout.Close()) })
			os.Stdout = stdout
			readStdout := func() string {
				data, err := os.ReadFile(stdoutPath)
				require.NoError(t, err)
				return string(data)
			}

			require.NoError(t, rootCmd.PersistentFlags().Lookup(tc.flag).Value.Set(tc.value))
			DispatchWait = tc.wait
			jsonMode := tc.value != "text"
			resultStatus := "succeeded"
			if tc.failed {
				resultStatus = "failed"
			}

			dispatchPolls, statusPolls := 0, 0
			mockAPI := &mocks.API{
				MockInitiateDispatch: func(cfg api.InitiateDispatchConfig) (*api.InitiateDispatchResult, error) {
					require.Equal(t, "test-dispatch", cfg.DispatchKey)
					return &api.InitiateDispatchResult{DispatchId: "dispatch-123"}, nil
				},
				MockGetDispatch: func(cfg api.GetDispatchConfig) (*api.GetDispatchResult, error) {
					require.Equal(t, "dispatch-123", cfg.DispatchId)
					if jsonMode {
						require.Empty(t, readStdout(), "stdout must remain empty while dispatch is starting")
					}
					dispatchPolls++
					if dispatchPolls == 1 {
						return &api.GetDispatchResult{Status: "not_ready"}, nil
					}
					return &api.GetDispatchResult{
						Status: "ready",
						Runs:   []api.GetDispatchRun{{RunID: "run-456", RunUrl: "https://example.com/run-456"}},
					}, nil
				},
				MockRunStatus: func(cfg api.RunStatusConfig) (api.RunStatusResult, error) {
					require.Equal(t, "run-456", cfg.RunID)
					if jsonMode {
						require.Empty(t, readStdout(), "stdout must remain empty while run is completing")
					}
					statusPolls++
					backoff := 0
					return api.RunStatusResult{
						Status:  &api.PollingRunStatus{Result: resultStatus},
						Polling: api.PollingResult{Completed: statusPolls == 2, BackoffMs: &backoff},
					}, nil
				},
				MockGetRunPrompt: func(string) (string, error) {
					return "Run prompt", nil
				},
				MockGetRunDetails: func(api.RunDetailsConfig) (map[string]any, error) {
					return map[string]any{}, nil
				},
			}
			service = cli.Service{Config: cli.Config{APIClient: mockAPI, Stdout: stdout, StdoutIsTTY: tc.tty}}

			err = dispatchCmd.RunE(dispatchCmd, []string{"test-dispatch"})
			if tc.failed {
				require.ErrorIs(t, err, HandledError)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, 2, dispatchPolls)
			if tc.wait {
				require.Equal(t, 2, statusPolls)
			} else {
				require.Zero(t, statusPolls)
			}

			output := readStdout()
			if jsonMode {
				if tc.wait {
					require.JSONEq(t, `{"RunID":"run-456","RunURL":"https://example.com/run-456","ResultStatus":"`+resultStatus+`","ResultPrompt":"Run prompt"}`, output)
				} else {
					require.JSONEq(t, `{"RunID":"run-456","RunURL":"https://example.com/run-456"}`, output)
				}
			} else {
				expected := "Waiting for dispatch to start...\n\nRun is watchable at https://example.com/run-456\n"
				if tc.wait {
					expected += "Waiting for run to complete...\n\nRun result status: succeeded\n\nRun prompt"
				}
				require.Equal(t, expected, output)
			}
		})
	}
}
