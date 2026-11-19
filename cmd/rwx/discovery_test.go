package main

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/mocks"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestOrganizationFlagIsLimitedToReadDiscovery(t *testing.T) {
	for _, cmd := range []*cobra.Command{resultsCmd, logsCmd, runsListCmd} {
		require.NotNil(t, cmd.Flags().Lookup("organization"), cmd.Name())
	}
	for _, cmd := range []*cobra.Command{rootCmd, runsCmd, runsCancelCmd} {
		require.Nil(t, cmd.Flags().Lookup("organization"), cmd.Name())
		require.Nil(t, cmd.PersistentFlags().Lookup("organization"), cmd.Name())
	}
}

func TestAmbiguousDefinitionSuggestionsKeepOrganization(t *testing.T) {
	err := HandleAmbiguousDefinitionPathError(&api.AmbiguousDefinitionPathError{
		Message: "Multiple definitions found", MatchingDefinitionPaths: []string{".rwx/ci.yml"},
	}, "main", "owner/repo", "public-org")
	require.ErrorContains(t, err, "rwx results --organization public-org --branch main --repo owner/repo --definition .rwx/ci.yml")
}

func TestReadCommandsTargetOrganization(t *testing.T) {
	for _, tc := range []struct {
		name      string
		cmd       *cobra.Command
		args      []string
		task      bool
		path      string
		discovery bool
	}{
		{name: "results discovery", cmd: resultsCmd, path: "/mint/api/results/latest", discovery: true},
		{name: "logs discovery", cmd: logsCmd, task: true, path: "/mint/api/results/latest", discovery: true},
		{name: "list discovery", cmd: runsListCmd, path: "/mint/api/runs", discovery: true},
		{name: "results exact", cmd: resultsCmd, args: []string{"run-id"}, path: "/mint/api/results/status"},
		{name: "logs exact task", cmd: logsCmd, args: []string{"task-id"}, path: "/mint/api/log_download"},
		{name: "logs exact run and key", cmd: logsCmd, args: []string{"run-id"}, task: true, path: "/mint/api/log_download"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			originalService := service
			t.Cleanup(func() { service = originalService })
			flag := tc.cmd.Flags().Lookup("organization")
			originalValue, originalChanged := flag.Value.String(), flag.Changed
			t.Cleanup(func() {
				require.NoError(t, flag.Value.Set(originalValue))
				flag.Changed = originalChanged
			})
			require.NoError(t, tc.cmd.Flags().Set("organization", "public-org"))
			if tc.task {
				taskFlag := tc.cmd.Flags().Lookup("task")
				value, changed := taskFlag.Value.String(), taskFlag.Changed
				t.Cleanup(func() { require.NoError(t, taskFlag.Value.Set(value)); taskFlag.Changed = changed })
				require.NoError(t, tc.cmd.Flags().Set("task", "build"))
			}
			calls := 0
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, tc.path, req.URL.Path)
				if tc.discovery {
					require.Equal(t, "public-org", req.URL.Query().Get("organization_slug"))
					require.Equal(t, "owner/project", req.URL.Query().Get("repository_name"))
				} else {
					require.NotContains(t, req.URL.Query(), "organization_slug")
				}
				return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(`{"error":"request reached cloud"}`))}, nil
			})
			service = cli.Service{Config: cli.Config{
				APIClient: client, VCSClient: &mocks.VCS{MockGetBranch: "main", MockGetOriginUrl: "git@github.com:owner/project.git"},
			}}
			err := tc.cmd.RunE(tc.cmd, tc.args)
			require.ErrorContains(t, err, "request reached cloud")
			require.Equal(t, 1, calls)
		})
	}
}
