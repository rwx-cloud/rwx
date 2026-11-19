package api_test

import (
	"net/http"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/stretchr/testify/require"
)

func TestAPIClient_DiscoveryOrganization(t *testing.T) {
	for _, organization := range []string{"", "public-org"} {
		t.Run("latest/"+organization, func(t *testing.T) {
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, http.MethodGet, req.Method)
				require.Equal(t, "/mint/api/results/latest", req.URL.Path)
				require.Equal(t, organization, req.URL.Query().Get("organization_slug"))
				require.Equal(t, "owner/repo", req.URL.Query().Get("repository_name"))
				require.Equal(t, "main", req.URL.Query().Get("branch_name"))
				require.Equal(t, "abc123", req.URL.Query().Get("commit_sha"))
				require.Equal(t, ".rwx/ci.yml", req.URL.Query().Get("definition_path"))
				if organization == "" {
					require.NotContains(t, req.URL.Query(), "organization_slug")
				}
				return &http.Response{StatusCode: 200, Body: jsonBody(t, api.RunStatusResult{RunID: "run-1"})}, nil
			})
			result, err := client.RunStatus(api.RunStatusConfig{
				OrganizationSlug: organization, RepositoryName: "owner/repo", BranchName: "main",
				CommitSha: "abc123", DefinitionPath: ".rwx/ci.yml",
			})
			require.NoError(t, err)
			require.Equal(t, "run-1", result.RunID)
		})
		t.Run("list/"+organization, func(t *testing.T) {
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, http.MethodGet, req.Method)
				require.Equal(t, "/mint/api/runs", req.URL.Path)
				require.Equal(t, organization, req.URL.Query().Get("organization_slug"))
				require.Equal(t, []string{"owner/repo", "other/repo"}, req.URL.Query()["repository_names[]"])
				if organization == "" {
					require.NotContains(t, req.URL.Query(), "organization_slug")
				}
				return &http.Response{StatusCode: 200, Body: jsonBody(t, api.ListRunsResult{})}, nil
			})
			_, err := client.ListRuns(api.ListRunsConfig{OrganizationSlug: organization, RepositoryNames: []string{"owner/repo", "other/repo"}})
			require.NoError(t, err)
		})
	}
}

func TestAPIClient_ExactStatusDoesNotTargetOrganization(t *testing.T) {
	for _, taskKey := range []string{"", "build"} {
		t.Run(taskKey, func(t *testing.T) {
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, "/mint/api/results/status", req.URL.Path)
				require.NotContains(t, req.URL.Query(), "organization_slug")
				require.NotContains(t, req.URL.Query(), "repository_name")
				if taskKey == "" {
					require.Equal(t, "exact-id", req.URL.Query().Get("id"))
				} else {
					require.Equal(t, "exact-id", req.URL.Query().Get("run_id"))
					require.Equal(t, taskKey, req.URL.Query().Get("task_key"))
				}
				return &http.Response{StatusCode: 200, Body: jsonBody(t, api.RunStatusResult{})}, nil
			})
			_, err := client.RunStatus(api.RunStatusConfig{OrganizationSlug: "ignored", RunID: "exact-id", TaskKey: taskKey})
			require.NoError(t, err)
		})
	}
}

func TestAPIClient_DiscoveryPreservesActionableServerError(t *testing.T) {
	client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 400, Body: jsonBody(t, map[string]string{"error": "Specify --organization <slug> to discover public runs"})}, nil
	})
	_, err := client.RunStatus(api.RunStatusConfig{RepositoryName: "repo", BranchName: "main"})
	require.ErrorContains(t, err, "Specify --organization <slug>")
	_, err = client.ListRuns(api.ListRunsConfig{})
	require.ErrorContains(t, err, "Specify --organization <slug>")
}
