package cli_test

import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/docker/cli/cli/config/types"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestService_ImagePull(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		wantPull bool
	}{
		{"older Cloud without configuration endpoint", api.ErrNotFound, true},
		{"configuration permission denied", fmt.Errorf("forbidden"), false},
		{"configuration retention expired", fmt.Errorf("retention expired"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := setupTest(t)
			s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) { return &api.WhoamiResult{OrganizationSlug: "org"}, nil }
			s.mockAPI.MockImagePullConfiguration = func(context.Context, string) (*api.ImageConfiguration, error) { return nil, tc.err }
			pulled := false
			s.mockDocker.PullFunc = func(context.Context, string, types.AuthConfig, bool) error { pulled = true; return nil }
			_, err := s.service.ImagePull(cli.ImagePullConfig{TaskID: "task", Timeout: time.Second})
			if tc.wantPull {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.err)
			}
			require.Equal(t, tc.wantPull, pulled)
		})
	}

	t.Run("loads manifest-only whiteouts locally and applies tags", func(t *testing.T) {
		s := setupTest(t)
		s.mockDocker.RegistryValue = "cloud.rwx.com"
		s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) { return &api.WhoamiResult{OrganizationSlug: "org"}, nil }
		s.mockAPI.MockImagePullConfiguration = func(ctx context.Context, id string) (*api.ImageConfiguration, error) {
			require.Equal(t, "task", id)
			return &api.ImageConfiguration{Architecture: "amd64", OS: "linux", Layers: []api.ImageLayer{{ID: "whiteout", Kind: "whiteout", Entries: []api.WhiteoutEntry{{FullPath: "/gone", Type: 3}}}}}, nil
		}
		s.mockDocker.PullFunc = func(context.Context, string, types.AuthConfig, bool) error {
			t.Fatal("must not use registry for manifest-only layers")
			return nil
		}
		loaded := false
		s.mockDocker.LoadFunc = func(ctx context.Context, input io.Reader, quiet bool) error {
			require.True(t, quiet)
			tr := tar.NewReader(input)
			for {
				h, err := tr.Next()
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
				if h.Name == "manifest.json" {
					var manifest []struct{ RepoTags []string }
					require.NoError(t, json.NewDecoder(tr).Decode(&manifest))
					require.Equal(t, []string{"cloud.rwx.com/org:task"}, manifest[0].RepoTags)
					loaded = true
				}
			}
			return nil
		}
		s.mockDocker.TagFunc = func(ctx context.Context, source, target string) error {
			require.Equal(t, "cloud.rwx.com/org:task", source)
			require.Equal(t, "local:test", target)
			return nil
		}
		result, err := s.service.ImagePull(cli.ImagePullConfig{TaskID: "task", Tags: []string{"local:test"}, Timeout: time.Second, OutputJSON: true})
		require.NoError(t, err)
		require.True(t, loaded)
		require.Equal(t, []string{"local:test"}, result.Tags)
		var output cli.ImagePullResult
		require.NoError(t, json.Unmarshal([]byte(s.mockStdout.String()), &output))
		require.Equal(t, result, &output)
	})

	t.Run("successful pull with no tags", func(t *testing.T) {
		s := setupTest(t)
		s.mockDocker.RegistryValue = "cloud.rwx.com"
		s.mockDocker.PasswordValue = "test-password"

		s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
			return &api.WhoamiResult{
				OrganizationSlug: "my-org",
			}, nil
		}

		s.mockDocker.PullFunc = func(ctx context.Context, imageRef string, authConfig types.AuthConfig, quiet bool) error {
			require.Equal(t, "cloud.rwx.com/my-org:task-456", imageRef)
			require.Equal(t, "my-org", authConfig.Username)
			require.Equal(t, "test-password", authConfig.Password)
			require.Equal(t, "cloud.rwx.com", authConfig.ServerAddress)
			return nil
		}

		cfg := cli.ImagePullConfig{
			TaskID:  "task-456",
			Tags:    []string{},
			Timeout: 1 * time.Second,
		}

		result, err := s.service.ImagePull(cfg)

		require.NoError(t, err)
		require.Equal(t, "cloud.rwx.com/my-org:task-456", result.ImageRef)
		require.Empty(t, result.Tags)
		require.Contains(t, s.mockStdout.String(), "Pulling image: cloud.rwx.com/my-org:task-456")
		require.Contains(t, s.mockStdout.String(), "Image pulled successfully!")
	})

	t.Run("successful pull with tags", func(t *testing.T) {
		s := setupTest(t)
		s.mockDocker.RegistryValue = "cloud.rwx.com"
		s.mockDocker.PasswordValue = "test-password"

		s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
			return &api.WhoamiResult{
				OrganizationSlug: "my-org",
			}, nil
		}

		s.mockDocker.PullFunc = func(ctx context.Context, imageRef string, authConfig types.AuthConfig, quiet bool) error {
			require.Equal(t, "cloud.rwx.com/my-org:task-456", imageRef)
			return nil
		}

		taggedRefs := []string{}
		s.mockDocker.TagFunc = func(ctx context.Context, source, target string) error {
			require.Equal(t, "cloud.rwx.com/my-org:task-456", source)
			taggedRefs = append(taggedRefs, target)
			return nil
		}

		cfg := cli.ImagePullConfig{
			TaskID:  "task-456",
			Tags:    []string{"latest", "v1.0.0"},
			Timeout: 1 * time.Second,
		}

		result, err := s.service.ImagePull(cfg)

		require.NoError(t, err)
		require.Equal(t, "cloud.rwx.com/my-org:task-456", result.ImageRef)
		require.Equal(t, []string{"latest", "v1.0.0"}, result.Tags)
		require.ElementsMatch(t, []string{
			"latest",
			"v1.0.0",
		}, taggedRefs)
		require.Contains(t, s.mockStdout.String(), "Tagging image as: latest")
		require.Contains(t, s.mockStdout.String(), "Tagging image as: v1.0.0")
	})

	t.Run("fails when config is invalid", func(t *testing.T) {
		s := setupTest(t)

		cfg := cli.ImagePullConfig{
			TaskID:  "",
			Timeout: 1 * time.Second,
		}

		result, err := s.service.ImagePull(cfg)

		require.Nil(t, result)
		require.Error(t, err)
		require.Contains(t, err.Error(), "task ID must be provided")
	})

	t.Run("fails when whoami fails", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
			return nil, fmt.Errorf("unauthorized")
		}

		cfg := cli.ImagePullConfig{
			TaskID:  "task-456",
			Timeout: 1 * time.Second,
		}

		result, err := s.service.ImagePull(cfg)

		require.Nil(t, result)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get organization info: unauthorized")
		require.Contains(t, err.Error(), "Try running `rwx login` again")
	})

	t.Run("fails when image pull fails", func(t *testing.T) {
		s := setupTest(t)
		s.mockDocker.RegistryValue = "cloud.rwx.com"
		s.mockDocker.PasswordValue = "test-password"

		s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
			return &api.WhoamiResult{
				OrganizationSlug: "my-org",
			}, nil
		}

		s.mockDocker.PullFunc = func(ctx context.Context, imageRef string, authConfig types.AuthConfig, quiet bool) error {
			return fmt.Errorf("failed to pull image: not found")
		}

		cfg := cli.ImagePullConfig{
			TaskID:  "task-456",
			Timeout: 1 * time.Second,
		}

		result, err := s.service.ImagePull(cfg)

		require.Nil(t, result)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to pull image")
		require.Contains(t, err.Error(), "not found")
	})

	t.Run("fails when pull times out", func(t *testing.T) {
		s := setupTest(t)
		s.mockDocker.RegistryValue = "cloud.rwx.com"
		s.mockDocker.PasswordValue = "test-password"

		s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
			return &api.WhoamiResult{
				OrganizationSlug: "my-org",
			}, nil
		}

		s.mockDocker.PullFunc = func(ctx context.Context, imageRef string, authConfig types.AuthConfig, quiet bool) error {
			return context.DeadlineExceeded
		}

		cfg := cli.ImagePullConfig{
			TaskID:  "task-456",
			Timeout: 1 * time.Second,
		}

		result, err := s.service.ImagePull(cfg)

		require.Nil(t, result)
		require.Error(t, err)
		require.Contains(t, err.Error(), "timeout while pulling image after 1s")
		require.Contains(t, err.Error(), "The image may still be available at: cloud.rwx.com/my-org:task-456")
	})

	t.Run("fails when image tagging fails", func(t *testing.T) {
		s := setupTest(t)
		s.mockDocker.RegistryValue = "cloud.rwx.com"
		s.mockDocker.PasswordValue = "test-password"

		s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
			return &api.WhoamiResult{
				OrganizationSlug: "my-org",
			}, nil
		}

		s.mockDocker.PullFunc = func(ctx context.Context, imageRef string, authConfig types.AuthConfig, quiet bool) error {
			return nil
		}

		s.mockDocker.TagFunc = func(ctx context.Context, source, target string) error {
			return fmt.Errorf("failed to tag image")
		}

		cfg := cli.ImagePullConfig{
			TaskID:  "task-456",
			Tags:    []string{"latest"},
			Timeout: 1 * time.Second,
		}

		result, err := s.service.ImagePull(cfg)

		require.Nil(t, result)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to tag image as latest")
	})

	t.Run("fails when tagging times out", func(t *testing.T) {
		s := setupTest(t)
		s.mockDocker.RegistryValue = "cloud.rwx.com"
		s.mockDocker.PasswordValue = "test-password"

		s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
			return &api.WhoamiResult{
				OrganizationSlug: "my-org",
			}, nil
		}

		s.mockDocker.PullFunc = func(ctx context.Context, imageRef string, authConfig types.AuthConfig, quiet bool) error {
			return nil
		}

		s.mockDocker.TagFunc = func(ctx context.Context, source, target string) error {
			return context.DeadlineExceeded
		}

		cfg := cli.ImagePullConfig{
			TaskID:  "task-456",
			Tags:    []string{"latest"},
			Timeout: 1 * time.Second,
		}

		result, err := s.service.ImagePull(cfg)

		require.Nil(t, result)
		require.Error(t, err)
		require.Contains(t, err.Error(), "timeout while tagging image after 1s")
	})

	t.Run("successful pull with JSON output", func(t *testing.T) {
		s := setupTest(t)
		s.mockDocker.RegistryValue = "cloud.rwx.com"
		s.mockDocker.PasswordValue = "test-password"

		s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
			return &api.WhoamiResult{
				OrganizationSlug: "my-org",
			}, nil
		}

		s.mockDocker.PullFunc = func(ctx context.Context, imageRef string, authConfig types.AuthConfig, quiet bool) error {
			require.Equal(t, "cloud.rwx.com/my-org:task-456", imageRef)
			require.True(t, quiet, "quiet should be true for JSON output")
			return nil
		}

		taggedRefs := []string{}
		s.mockDocker.TagFunc = func(ctx context.Context, source, target string) error {
			taggedRefs = append(taggedRefs, target)
			return nil
		}

		cfg := cli.ImagePullConfig{
			TaskID:     "task-456",
			Tags:       []string{"latest", "v1.0.0"},
			Timeout:    1 * time.Second,
			OutputJSON: true,
		}

		result, err := s.service.ImagePull(cfg)

		require.NoError(t, err)
		require.Equal(t, "cloud.rwx.com/my-org:task-456", result.ImageRef)
		require.Equal(t, []string{"latest", "v1.0.0"}, result.Tags)
		require.ElementsMatch(t, []string{"latest", "v1.0.0"}, taggedRefs)

		// Verify no human-readable output
		require.NotContains(t, s.mockStdout.String(), "Pulling image:")
		require.NotContains(t, s.mockStdout.String(), "Image pulled successfully!")
		require.NotContains(t, s.mockStdout.String(), "Tagging image as:")

		// Verify JSON output
		require.Contains(t, s.mockStdout.String(), `"ImageRef":"cloud.rwx.com/my-org:task-456"`)
		require.Contains(t, s.mockStdout.String(), `"Tags":["latest","v1.0.0"]`)
	})

	t.Run("JSON output with no tags", func(t *testing.T) {
		s := setupTest(t)
		s.mockDocker.RegistryValue = "cloud.rwx.com"
		s.mockDocker.PasswordValue = "test-password"

		s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
			return &api.WhoamiResult{
				OrganizationSlug: "my-org",
			}, nil
		}

		s.mockDocker.PullFunc = func(ctx context.Context, imageRef string, authConfig types.AuthConfig, quiet bool) error {
			require.True(t, quiet)
			return nil
		}

		cfg := cli.ImagePullConfig{
			TaskID:     "task-456",
			Tags:       []string{},
			Timeout:    1 * time.Second,
			OutputJSON: true,
		}

		result, err := s.service.ImagePull(cfg)

		require.NoError(t, err)
		require.Equal(t, "cloud.rwx.com/my-org:task-456", result.ImageRef)
		require.Empty(t, result.Tags)
		require.Contains(t, s.mockStdout.String(), `"ImageRef":"cloud.rwx.com/my-org:task-456"`)
		// Tags should be omitted if empty (omitempty)
		require.NotContains(t, s.mockStdout.String(), `"tags"`)
	})
}
