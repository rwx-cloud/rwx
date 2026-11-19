package cli_test

import (
	"os/exec"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/git"
	"github.com/stretchr/testify/require"
)

func TestDiscoveryRepositoryIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, origin, repository, organization, expected string
		invalid                                          bool
	}{
		{name: "legacy basename", origin: "git@github.com:owner/repo.git", expected: "repo"},
		{name: "SSH", origin: "git@github.com:owner/repo.git", organization: "public-org", expected: "owner/repo"},
		{name: "HTTPS", origin: "https://github.com/owner/repo.git", organization: "public-org", expected: "owner/repo"},
		{name: "SSH URL with port", origin: "ssh://git@github.com:2222/owner/repo.git", organization: "public-org", expected: "owner/repo"},
		{name: "GitLab subgroup", origin: "git@gitlab.com:owner/subgroup/repo.git", organization: "public-org", expected: "owner/subgroup/repo"},
		{name: "invalid nested path", repository: "owner/../repo", organization: "public-org", invalid: true},
		{name: "explicit repository", repository: "different/project", organization: "public-org", expected: "different/project"},
		{name: "legacy explicit basename", repository: "repo", expected: "repo"},
		{name: "explicit basename rejected", repository: "repo", organization: "public-org", invalid: true},
		{name: "no origin", organization: "public-org", invalid: true},
		{name: "local origin", origin: "/local/repo.git", organization: "public-org", invalid: true},
		{name: "missing owner", origin: "https://github.com/repo.git", organization: "public-org", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setup := setupTest(t)
			setup.mockVCS.MockGetBranch = "main"
			setup.mockVCS.MockGetOriginUrl = tc.origin
			calls := 0
			setup.mockAPI.MockRunStatus = func(cfg api.RunStatusConfig) (api.RunStatusResult, error) {
				calls++
				require.Equal(t, tc.organization, cfg.OrganizationSlug)
				require.Equal(t, tc.expected, cfg.RepositoryName)
				require.Equal(t, "main", cfg.BranchName)
				return api.RunStatusResult{RunID: "run-1"}, nil
			}
			_, err := setup.service.ResolveRunIDFromGitContext(cli.ResolveRunIDConfig{OrganizationSlug: tc.organization, RepositoryName: tc.repository})
			if tc.invalid {
				require.ErrorContains(t, err, "owner/repo")
				require.Zero(t, calls)
			} else {
				require.NoError(t, err)
				require.Equal(t, 1, calls)
			}
		})
	}
}

func TestListRunsOrganizationRepositories(t *testing.T) {
	for _, tc := range []struct {
		name, organization     string
		repositories, expected []string
		invalid                bool
	}{
		{name: "legacy organization wide"},
		{name: "legacy basename", repositories: []string{"repo"}, expected: []string{"repo"}},
		{name: "inferred", organization: "public-org", expected: []string{"owner/repo"}},
		{name: "explicit", organization: "public-org", repositories: []string{"a/repo", "b/repo"}, expected: []string{"a/repo", "b/repo"}},
		{name: "invalid", organization: "public-org", repositories: []string{"repo"}, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setup := setupTest(t)
			setup.mockVCS.MockGetOriginUrl = "https://github.com/owner/repo.git"
			calls := 0
			setup.mockAPI.MockListRuns = func(cfg api.ListRunsConfig) (*api.ListRunsResult, error) {
				calls++
				require.Equal(t, tc.organization, cfg.OrganizationSlug)
				require.Equal(t, tc.expected, cfg.RepositoryNames)
				return &api.ListRunsResult{}, nil
			}
			_, err := setup.service.ListRuns(cli.ListRunsConfig{OrganizationSlug: tc.organization, RepositoryNames: tc.repositories})
			if tc.invalid {
				require.ErrorContains(t, err, "owner/repo")
				require.Zero(t, calls)
			} else {
				require.NoError(t, err)
				require.Equal(t, 1, calls)
			}
		})
	}
}

func TestDiscoveryUsesGitOriginNotCheckoutDirectory(t *testing.T) {
	setup := setupTest(t)
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "--initial-branch=main"}, {"remote", "add", "origin", "git@github.com:canonical-owner/canonical-repo.git"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", output)
	}
	setup.service.VCSClient = &git.Client{Binary: "git", Dir: dir}
	setup.mockAPI.MockRunStatus = func(cfg api.RunStatusConfig) (api.RunStatusResult, error) {
		require.Equal(t, "canonical-owner/canonical-repo", cfg.RepositoryName)
		require.Equal(t, "public-org", cfg.OrganizationSlug)
		return api.RunStatusResult{RunID: "run-1"}, nil
	}
	runID, err := setup.service.ResolveRunIDFromGitContext(cli.ResolveRunIDConfig{OrganizationSlug: "public-org", BranchName: "main"})
	require.NoError(t, err)
	require.Equal(t, "run-1", runID)
}

func TestGetRunStatusPassesDiscoveryOrganization(t *testing.T) {
	setup := setupTest(t)
	setup.mockAPI.MockRunStatus = func(cfg api.RunStatusConfig) (api.RunStatusResult, error) {
		require.Equal(t, "public-org", cfg.OrganizationSlug)
		require.Equal(t, "owner/repo", cfg.RepositoryName)
		require.Equal(t, "main", cfg.BranchName)
		return api.RunStatusResult{
			RunID: "run-1", Status: &api.PollingRunStatus{Result: "succeeded"},
			Polling: api.PollingResult{Completed: true},
		}, nil
	}
	result, err := setup.service.GetRunStatus(cli.GetRunStatusConfig{
		OrganizationSlug: "public-org", RepositoryName: "owner/repo", BranchName: "main",
	})
	require.NoError(t, err)
	require.Equal(t, "run-1", result.RunID)
}
