package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestTaskAttemptCommands(t *testing.T) {
	require.NotNil(t, resultsCmd.Flags().Lookup("attempt"))
	require.NotNil(t, logsCmd.Flags().Lookup("attempt"))
	for _, command := range []string{"results", "logs"} {
		for _, taskKey := range []string{"", "ci.build"} {
			for _, attempt := range []string{"", "2"} {
				t.Run(fmt.Sprintf("%s/key=%s/attempt=%s", command, taskKey, attempt), func(t *testing.T) {
					originalService, originalFormat := service, Format
					originalTask, originalAttempt, originalWait := ResultsTaskKey, ResultsAttempt, ResultsWait
					originalLogsTask, originalLogsAttempt, originalOutput, originalZip := LogsTaskKey, LogsAttempt, LogsOutput, LogsZip
					originalStdout := os.Stdout
					t.Cleanup(func() {
						service, Format = originalService, originalFormat
						ResultsTaskKey, ResultsAttempt, ResultsWait = originalTask, originalAttempt, originalWait
						LogsTaskKey, LogsAttempt, LogsOutput, LogsZip = originalLogsTask, originalLogsAttempt, originalOutput, originalZip
						os.Stdout = originalStdout
					})

					inputID, selectedID := "task-original", "task-original"
					if taskKey != "" {
						inputID, selectedID = "run-123", "task-latest"
					}
					if attempt != "" {
						selectedID = "task-attempt-2"
					}
					archive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						fmt.Fprint(w, "selected attempt archive")
					}))
					defer archive.Close()
					statusCalls, summaryCalls, promptCalls, detailCalls, logCalls := 0, 0, 0, 0, 0
					client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
						query := req.URL.Query()
						body := ""
						if query.Get("view") == "summary" {
							summaryCalls++
							require.Equal(t, "/mint/api/results/"+inputID, req.URL.Path)
							require.Equal(t, taskKey, query.Get("task_key"))
							require.Equal(t, "2", query.Get("attempt_number"))
							body = `{"id":"task-attempt-2","attempt_number":2,"latest_attempt_number":3}`
						} else {
							require.Empty(t, query.Get("attempt_number"))
							wantID, wantKey := inputID, taskKey
							if attempt != "" {
								wantID, wantKey = selectedID, ""
							}
							switch req.URL.Path {
							case "/mint/api/results/status":
								statusCalls++
								if wantKey != "" {
									require.Equal(t, wantID, query.Get("run_id"))
								} else {
									require.Equal(t, wantID, query.Get("id"))
								}
								require.Equal(t, wantKey, query.Get("task_key"))
								body = fmt.Sprintf(`{"run_id":"run-123","task_id":%q,"task_status":{"result":"succeeded"},"polling":{"completed":%t,"backoff_ms":0}}`, selectedID, statusCalls == 2)
							case "/mint/api/results/prompt":
								promptCalls++
								require.Equal(t, "cli", req.Header.Get("X-RWX-Prompt-Presentation"))
								if wantKey != "" {
									require.Equal(t, inputID, query.Get("run_id"))
								} else {
									require.Equal(t, selectedID, query.Get("id"))
								}
								require.Equal(t, wantKey, query.Get("task_key"))
								body = "selected attempt prompt"
							case "/mint/api/results/" + wantID:
								detailCalls++
								require.Empty(t, query.Get("view"))
								require.Equal(t, wantKey, query.Get("task_key"))
								body = fmt.Sprintf(`{"id":%q}`, selectedID)
							case "/mint/api/log_download":
								logCalls++
								if wantKey != "" {
									require.Equal(t, wantID, query.Get("run_id"))
								} else {
									require.Equal(t, wantID, query.Get("id"))
								}
								require.Equal(t, wantKey, query.Get("task_key"))
								body = fmt.Sprintf(`{"url":%q,"filename":"logs.zip","run_id":"run-123"}`, archive.URL)
							default:
								t.Fatalf("unexpected request: %s", req.URL)
							}
						}
						return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
					})
					service = cli.Service{Config: cli.Config{APIClient: client, Stdout: io.Discard, Stderr: io.Discard}}
					Format = "json"
					cmd := &cobra.Command{}
					cmd.Flags().String("task", "", "")
					cmd.Flags().String("attempt", "", "")
					cmd.Flags().String("output", "", "")
					if taskKey != "" {
						require.NoError(t, cmd.Flags().Set("task", taskKey))
					}
					if attempt != "" {
						require.NoError(t, cmd.Flags().Set("attempt", attempt))
					}
					if command == "results" {
						ResultsTaskKey, ResultsAttempt, ResultsWait = taskKey, attempt, true
						output, err := os.CreateTemp(t.TempDir(), "results")
						require.NoError(t, err)
						defer output.Close()
						os.Stdout = output
						require.NoError(t, resultsCmd.RunE(cmd, []string{inputID}))
						data, err := os.ReadFile(output.Name())
						require.NoError(t, err)
						require.Contains(t, string(data), `"TaskID":"`+selectedID+`"`)
						require.Contains(t, string(data), `"ResultPrompt":"selected attempt prompt"`)
						require.Equal(t, 2, statusCalls)
						require.Equal(t, 1, promptCalls)
						require.Equal(t, 1, detailCalls)
					} else {
						LogsTaskKey, LogsAttempt, LogsZip = taskKey, attempt, true
						LogsOutput = filepath.Join(t.TempDir(), "logs.zip")
						require.NoError(t, cmd.Flags().Set("output", LogsOutput))
						require.NoError(t, logsCmd.RunE(cmd, []string{inputID}))
						data, err := os.ReadFile(LogsOutput)
						require.NoError(t, err)
						require.Equal(t, "selected attempt archive", string(data))
						require.Equal(t, 1, logCalls)
					}
					if attempt != "" {
						require.Equal(t, 1, summaryCalls)
					} else {
						require.Zero(t, summaryCalls)
					}
				})
			}
		}
	}
}

func TestTaskAttemptCommandsRejectInvalidSelection(t *testing.T) {
	for _, command := range []*cobra.Command{resultsCmd, logsCmd} {
		for _, tc := range []struct {
			name    string
			attempt string
			args    []string
			key     string
			status  int
			body    string
			want    string
		}{
			{name: "no task", attempt: "2", want: "task ID or --task"},
			{name: "invalid", attempt: "0", args: []string{"task-original"}, want: "positive integer"},
			{name: "empty", args: []string{"task-original"}, want: "positive integer"},
			{name: "run only", attempt: "2", args: []string{"run-123"}, status: 400, body: `{"error":"attempt_number requires a task ID or scoped task key."}`, want: "requires a task ID"},
			{name: "missing", attempt: "2", args: []string{"task-original"}, status: 404, body: `{"error":"Not found"}`, want: "Not found"},
			{name: "wrong attempt", attempt: "2", args: []string{"task-original"}, status: 200, body: `{"id":"task-latest","attempt_number":3}`, want: "requested task attempt was not returned"},
			{name: "ambiguous key", attempt: "2", args: []string{"run-123"}, key: "build", status: 422, body: `{"error":"The task key 'build' is ambiguous"}`, want: "ambiguous"},
		} {
			t.Run(command.Name()+"/"+tc.name, func(t *testing.T) {
				originalService := service
				originalTask, originalAttempt := ResultsTaskKey, ResultsAttempt
				originalLogsTask, originalLogsAttempt := LogsTaskKey, LogsAttempt
				t.Cleanup(func() {
					service = originalService
					ResultsTaskKey, ResultsAttempt = originalTask, originalAttempt
					LogsTaskKey, LogsAttempt = originalLogsTask, originalLogsAttempt
				})
				calls := 0
				client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
					calls++
					require.Equal(t, "summary", req.URL.Query().Get("view"))
					return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
				})
				service = cli.Service{Config: cli.Config{APIClient: client}}
				ResultsTaskKey, ResultsAttempt = tc.key, tc.attempt
				LogsTaskKey, LogsAttempt = tc.key, tc.attempt
				cmd := &cobra.Command{}
				cmd.Flags().String("attempt", "", "")
				cmd.Flags().String("task", "", "")
				require.NoError(t, cmd.Flags().Set("attempt", tc.attempt))
				if tc.key != "" {
					require.NoError(t, cmd.Flags().Set("task", tc.key))
				}
				err := command.RunE(cmd, tc.args)
				require.ErrorContains(t, err, tc.want)
				if tc.status == 0 {
					require.Zero(t, calls)
				} else {
					require.Equal(t, 1, calls)
				}
				if tc.status == 404 {
					require.ErrorIs(t, err, api.ErrNotFound)
				}
			})
		}
	}
}
