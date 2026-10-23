package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/mocks"
	"github.com/stretchr/testify/require"
)

func TestResultsCompactOutput(t *testing.T) {
	for _, verbose := range []bool{false, true} {
		name := "default compact"
		if verbose {
			name = "verbose legacy"
		}
		t.Run(name, func(t *testing.T) {
			originalService, originalStdout := service, os.Stdout
			originalFormat, originalVerbose := Format, ResultsVerbose
			t.Cleanup(func() {
				service, os.Stdout = originalService, originalStdout
				Format, ResultsVerbose = originalFormat, originalVerbose
			})
			path := filepath.Join(t.TempDir(), "stdout")
			stdout, err := os.Create(path)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, stdout.Close()) })
			os.Stdout = stdout
			Format, ResultsVerbose = "json", verbose
			mockAPI := &mocks.API{
				MockRunStatus: func(cfg api.RunStatusConfig) (api.RunStatusResult, error) {
					require.Equal(t, "run-id", cfg.RunID)
					return api.RunStatusResult{Status: &api.PollingRunStatus{Result: "succeeded"}, Polling: api.PollingResult{Completed: true}}, nil
				},
				MockGetRunPrompt: func(id string) (string, error) {
					require.Equal(t, "run-id", id)
					return "unchanged prompt", nil
				},
				MockGetRunDetails: func(cfg api.RunDetailsConfig) (map[string]any, error) {
					require.Equal(t, !verbose, cfg.Compact)
					require.Equal(t, "run-id", cfg.RunID)
					details := map[string]any{"id": "run-id", "repository_name": nil, "tasks": []any{map[string]any{"id": "child", "attempt_number": float64(2)}}}
					if verbose {
						details["env"] = map[string]any{"GIT_FLAG": "on"}
					}
					return details, nil
				},
			}
			service = cli.Service{Config: cli.Config{APIClient: mockAPI, Stdout: stdout}}
			require.NoError(t, resultsCmd.RunE(resultsCmd, []string{"run-id"}))
			body, err := os.ReadFile(path)
			require.NoError(t, err)
			var output map[string]any
			require.NoError(t, json.Unmarshal(body, &output))
			require.Equal(t, "run-id", output["RunID"])
			require.Equal(t, "run-id", output["ID"])
			require.Equal(t, "unchanged prompt", output["ResultPrompt"])
			require.Equal(t, "succeeded", output["ResultStatus"])
			require.Equal(t, true, output["Completed"])
			require.Contains(t, output, "RepositoryName")
			require.Nil(t, output["RepositoryName"])
			require.Equal(t, []any{map[string]any{"ID": "child", "AttemptNumber": float64(2)}}, output["Tasks"])
			if !verbose {
				require.NotContains(t, output, "Env")
			} else {
				require.Equal(t, map[string]any{"GIT_FLAG": "on"}, output["Env"])
			}
		})
	}
}

func TestResultsVerboseRequiresJSON(t *testing.T) {
	originalFormat, originalVerbose := Format, ResultsVerbose
	t.Cleanup(func() { Format, ResultsVerbose = originalFormat, originalVerbose })
	Format, ResultsVerbose = "text", true
	require.ErrorContains(t, resultsCmd.RunE(resultsCmd, []string{"run-id"}), "--verbose requires --json or --format json")
}
