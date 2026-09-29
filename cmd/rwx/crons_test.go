package main

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestCronsCommands(t *testing.T) {
	require.Same(t, cronsCmd, findSubcommand(rootCmd, "crons"))
	require.Len(t, cronsCmd.Commands(), 5)
	for _, name := range []string{"show", "pause", "resume", "snooze"} {
		cmd := findSubcommand(cronsCmd, name)
		require.NotNil(t, cmd)
		require.Error(t, cmd.Args(cmd, nil))
		require.NoError(t, cmd.Args(cmd, []string{"cron-1"}))
		require.Error(t, cmd.Args(cmd, []string{"cron-1", "cron-2"}))
		require.NoError(t, cmd.Flags().Set("id", "cron-1"))
		require.NoError(t, cmd.Args(cmd, nil))
		require.Error(t, cmd.Args(cmd, []string{"nightly"}))
		require.NoError(t, cmd.Flags().Set("id", ""))
	}
	require.NoError(t, cronsListCmd.Args(cronsListCmd, nil))
	require.Error(t, cronsListCmd.Args(cronsListCmd, []string{"cron-1"}))
	require.ErrorContains(t, cronsSnoozeCmd.ValidateRequiredFlags(), "until")
}

func TestCronsCommandOutput(t *testing.T) {
	for _, action := range []string{"list", "show", "pause", "resume", "snooze"} {
		t.Run(action, func(t *testing.T) {
			originalService, originalFormat := service, Format
			t.Cleanup(func() {
				service, Format = originalService, originalFormat
			})
			Format = "json"
			var stdout bytes.Buffer
			called := false
			listed := false
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				if action != "list" && req.URL.Path == "/mint/api/crons" {
					listed = true
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"crons":[{"id":"cron-1","key":"nightly"}]}`))}, nil
				}
				called = true
				path := "/mint/api/crons/cron-1"
				status := http.StatusOK
				body := `{"cron":{"id":"cron-1","status":"active"}}`
				switch action {
				case "list":
					path = "/mint/api/crons"
					body = `{"crons":[]}`
				case "show":
				default:
					path += "/" + action
				}
				require.Equal(t, path, req.URL.Path)
				if action == "snooze" {
					body, err := io.ReadAll(req.Body)
					require.NoError(t, err)
					require.JSONEq(t, `{"until":"2027-01-02T09:30:59-05:00"}`, string(body))
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			service = cli.Service{Config: cli.Config{APIClient: client, Stdout: &stdout}}
			cmd := findSubcommand(cronsCmd, action)
			if action == "snooze" {
				require.NoError(t, cmd.Flags().Set("until", "2027-01-02T09:30:59-05:00"))
				t.Cleanup(func() {
					require.NoError(t, cmd.Flags().Set("until", ""))
					cmd.Flags().Lookup("until").Changed = false
				})
			}
			args := []string{"nightly"}
			if action == "list" {
				args = nil
			}
			require.NoError(t, cmd.RunE(cmd, args))
			require.True(t, called)
			require.Equal(t, action != "list", listed)
			switch action {
			case "list":
				require.JSONEq(t, `{"Crons":[]}`, stdout.String())
			default:
				require.Contains(t, stdout.String(), `"Cron":{"ID":"cron-1"`)
			}
		})
	}
}

func TestCronTargetFlags(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want cli.CronConfig
	}{
		{name: "key and filters", args: []string{"nightly", "--repository", "org/app", "--branch", "release", "--file", ".rwx/ci.yml"}, want: cli.CronConfig{Key: "nightly", Repository: "org/app", Branch: "release", File: ".rwx/ci.yml"}},
		{name: "ID", args: []string{"--id", "cron-1"}, want: cli.CronConfig{ID: "cron-1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			originalFormat := Format
			Format = "text"
			t.Cleanup(func() { Format = originalFormat })
			called := false
			cmd := newCronCommand("pause [KEY]", "Pause a cron", func(cfg cli.CronConfig) (*cli.CronResult, error) {
				called = true
				require.Equal(t, tc.want, cfg)
				return &cli.CronResult{}, nil
			})
			cmd.SetArgs(tc.args)
			require.NoError(t, cmd.Execute())
			require.True(t, called)
		})
	}
}
