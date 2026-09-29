package cli_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/docker/cli/cli/config/types"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

func setupImageBuildTest(t *testing.T, s *testSetup) {
	err := os.WriteFile("test.yml", []byte("base:\n  os: ubuntu 24.04\n  tag: 1.0\ntasks:\n  - key: build-task\n    run: echo 'building'\n"), 0o644)
	require.NoError(t, err)

	s.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
		return api.DefaultBaseResult{Image: "ubuntu:24.04", Config: "rwx/base 1.0.0", Arch: "x86_64"}, nil
	}

	s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
		return &api.PackageVersionsResult{
			LatestMajor: map[string]string{},
			LatestMinor: map[string]map[string]string{},
		}, nil
	}

	s.mockVCS.MockGetBranch = "main"
	s.mockVCS.MockGetCommit = "abc123"
	s.mockVCS.MockGetOriginUrl = "git@github.com:test/test.git"
}

func TestService_ImageBuild(t *testing.T) {
	t.Run("successful build with no tags", func(t *testing.T) {
		s := setupTest(t)
		setupImageBuildTest(t, s)
		s.mockDocker.RegistryValue = "cloud.rwx.com"
		s.mockDocker.PasswordValue = "test-password"

		s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			require.Equal(t, "test.yml", cfg.TaskDefinitions[0].Path)
			require.True(t, cfg.UseCache)
			require.Equal(t, []string{"build-task"}, cfg.TargetedTaskKeys)

			return &api.InitiateRunResult{
				RunID:  "run-123",
				RunURL: "https://cloud.rwx.com/runs/run-123",
			}, nil
		}

		callCount := 0
		s.mockAPI.MockTaskKeyStatus = func(cfg api.TaskKeyStatusConfig) (api.TaskStatusResult, error) {
			require.Equal(t, "run-123", cfg.RunID)
			require.Equal(t, "build-task", cfg.TaskKey)

			callCount++
			if callCount == 1 {
				backoffMs := 0
				return api.TaskStatusResult{
					Status: &api.TaskStatus{Result: "no_result"},
					Polling: api.PollingResult{
						Completed: false,
						BackoffMs: &backoffMs,
					},
				}, nil
			}

			return api.TaskStatusResult{
				Status: &api.TaskStatus{Result: api.TaskStatusSucceeded},
				TaskID: "task-456",
				Polling: api.PollingResult{
					Completed: true,
				},
			}, nil
		}

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

		cfg := cli.ImageBuildConfig{
			InitParameters: map[string]string{"key": "value"},
			MintFilePath:   "test.yml",
			NoCache:        false,
			TargetTaskKey:  "build-task",
			Tags:           []string{},
			Timeout:        1 * time.Second,
			OpenURL:        func(string) error { return nil },
		}

		_, err := s.service.ImageBuild(cfg)

		require.NoError(t, err)
		require.Contains(t, s.mockStdout.String(), "Building image for build-task")
		require.Contains(t, s.mockStdout.String(), "Run URL: https://cloud.rwx.com/runs/run-123")
		require.Contains(t, s.mockStdout.String(), "Build succeeded!")
		require.Contains(t, s.mockStdout.String(), "Pulling image: cloud.rwx.com/my-org:task-456")
		require.Contains(t, s.mockStdout.String(), "Image pulled successfully!")
	})

	t.Run("successful build with tags", func(t *testing.T) {
		s := setupTest(t)
		setupImageBuildTest(t, s)
		s.mockDocker.RegistryValue = "cloud.rwx.com"
		s.mockDocker.PasswordValue = "test-password"

		s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			return &api.InitiateRunResult{
				RunID:  "run-123",
				RunURL: "https://cloud.rwx.com/runs/run-123",
			}, nil
		}

		s.mockAPI.MockTaskKeyStatus = func(cfg api.TaskKeyStatusConfig) (api.TaskStatusResult, error) {
			return api.TaskStatusResult{
				Status: &api.TaskStatus{Result: api.TaskStatusSucceeded},
				TaskID: "task-456",
				Polling: api.PollingResult{
					Completed: true,
				},
			}, nil
		}

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

		cfg := cli.ImageBuildConfig{
			InitParameters: map[string]string{"key": "value"},
			MintFilePath:   "test.yml",
			NoCache:        false,
			TargetTaskKey:  "build-task",
			Tags:           []string{"latest", "v1.0.0"},
			Timeout:        1 * time.Second,
			OpenURL:        func(string) error { return nil },
		}

		_, err := s.service.ImageBuild(cfg)

		require.NoError(t, err)
		require.ElementsMatch(t, []string{
			"latest",
			"v1.0.0",
		}, taggedRefs)
		require.Contains(t, s.mockStdout.String(), "Tagging image as: latest")
		require.Contains(t, s.mockStdout.String(), "Tagging image as: v1.0.0")
	})

	t.Run("fails when run initiation fails", func(t *testing.T) {
		s := setupTest(t)
		setupImageBuildTest(t, s)

		s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			return nil, fmt.Errorf("failed to initiate run")
		}

		cfg := cli.ImageBuildConfig{
			InitParameters: map[string]string{},
			MintFilePath:   "test.yml",
			TargetTaskKey:  "build-task",
			Timeout:        1 * time.Second,
			OpenURL:        func(string) error { return nil },
		}

		_, err := s.service.ImageBuild(cfg)

		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to initiate run")
	})

	t.Run("fails when task status polling fails", func(t *testing.T) {
		s := setupTest(t)
		setupImageBuildTest(t, s)

		s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			return &api.InitiateRunResult{
				RunID:  "run-123",
				RunURL: "https://cloud.rwx.com/runs/run-123",
			}, nil
		}

		s.mockAPI.MockTaskKeyStatus = func(cfg api.TaskKeyStatusConfig) (api.TaskStatusResult, error) {
			return api.TaskStatusResult{}, fmt.Errorf("failed to get task status")
		}

		cfg := cli.ImageBuildConfig{
			InitParameters: map[string]string{},
			MintFilePath:   "test.yml",
			TargetTaskKey:  "build-task",
			Timeout:        1 * time.Second,
			OpenURL:        func(string) error { return nil },
		}

		_, err := s.service.ImageBuild(cfg)

		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get build status")
		require.Contains(t, err.Error(), "failed to get task status")
	})

	t.Run("fails when build fails", func(t *testing.T) {
		s := setupTest(t)
		setupImageBuildTest(t, s)

		s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			return &api.InitiateRunResult{
				RunID:  "run-123",
				RunURL: "https://cloud.rwx.com/runs/run-123",
			}, nil
		}

		s.mockAPI.MockTaskKeyStatus = func(cfg api.TaskKeyStatusConfig) (api.TaskStatusResult, error) {
			return api.TaskStatusResult{
				Status: &api.TaskStatus{Result: "failed"},
				Polling: api.PollingResult{
					Completed: true,
				},
			}, nil
		}

		cfg := cli.ImageBuildConfig{
			InitParameters: map[string]string{},
			MintFilePath:   "test.yml",
			TargetTaskKey:  "build-task",
			Timeout:        1 * time.Second,
			OpenURL:        func(string) error { return nil },
		}

		_, err := s.service.ImageBuild(cfg)

		require.Error(t, err)
		require.Equal(t, "build failed", err.Error())
	})

	t.Run("fails when build fails without error message", func(t *testing.T) {
		s := setupTest(t)
		setupImageBuildTest(t, s)

		s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			return &api.InitiateRunResult{
				RunID:  "run-123",
				RunURL: "https://cloud.rwx.com/runs/run-123",
			}, nil
		}

		s.mockAPI.MockTaskKeyStatus = func(cfg api.TaskKeyStatusConfig) (api.TaskStatusResult, error) {
			return api.TaskStatusResult{
				Status: &api.TaskStatus{Result: "failed"},
				Polling: api.PollingResult{
					Completed: true,
				},
			}, nil
		}

		cfg := cli.ImageBuildConfig{
			InitParameters: map[string]string{},
			MintFilePath:   "test.yml",
			TargetTaskKey:  "build-task",
			Timeout:        1 * time.Second,
			OpenURL:        func(string) error { return nil },
		}

		_, err := s.service.ImageBuild(cfg)

		require.Error(t, err)
		require.Equal(t, "build failed", err.Error())
	})

	t.Run("fails when whoami fails", func(t *testing.T) {
		s := setupTest(t)
		setupImageBuildTest(t, s)

		s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			return &api.InitiateRunResult{
				RunID:  "run-123",
				RunURL: "https://cloud.rwx.com/runs/run-123",
			}, nil
		}

		s.mockAPI.MockTaskKeyStatus = func(cfg api.TaskKeyStatusConfig) (api.TaskStatusResult, error) {
			return api.TaskStatusResult{
				Status: &api.TaskStatus{Result: api.TaskStatusSucceeded},
				TaskID: "task-456",
				Polling: api.PollingResult{
					Completed: true,
				},
			}, nil
		}

		s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
			return nil, fmt.Errorf("unauthorized")
		}

		cfg := cli.ImageBuildConfig{
			InitParameters: map[string]string{},
			MintFilePath:   "test.yml",
			TargetTaskKey:  "build-task",
			Timeout:        1 * time.Second,
			OpenURL:        func(string) error { return nil },
		}

		_, err := s.service.ImageBuild(cfg)

		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get organization info: unauthorized")
		require.Contains(t, err.Error(), "Try running `rwx login` again")
	})

	t.Run("fails when image pull fails", func(t *testing.T) {
		s := setupTest(t)
		setupImageBuildTest(t, s)
		s.mockDocker.RegistryValue = "cloud.rwx.com"
		s.mockDocker.PasswordValue = "test-password"

		s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			return &api.InitiateRunResult{
				RunID:  "run-123",
				RunURL: "https://cloud.rwx.com/runs/run-123",
			}, nil
		}

		s.mockAPI.MockTaskKeyStatus = func(cfg api.TaskKeyStatusConfig) (api.TaskStatusResult, error) {
			return api.TaskStatusResult{
				Status: &api.TaskStatus{Result: api.TaskStatusSucceeded},
				TaskID: "task-456",
				Polling: api.PollingResult{
					Completed: true,
				},
			}, nil
		}

		s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
			return &api.WhoamiResult{
				OrganizationSlug: "my-org",
			}, nil
		}

		s.mockDocker.PullFunc = func(ctx context.Context, imageRef string, authConfig types.AuthConfig, quiet bool) error {
			return fmt.Errorf("failed to pull image: not found")
		}

		cfg := cli.ImageBuildConfig{
			InitParameters: map[string]string{},
			MintFilePath:   "test.yml",
			TargetTaskKey:  "build-task",
			Timeout:        1 * time.Second,
			OpenURL:        func(string) error { return nil },
		}

		_, err := s.service.ImageBuild(cfg)

		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to pull image")
		require.Contains(t, err.Error(), "not found")
	})

	t.Run("fails when image tagging fails", func(t *testing.T) {
		s := setupTest(t)
		setupImageBuildTest(t, s)
		s.mockDocker.RegistryValue = "cloud.rwx.com"
		s.mockDocker.PasswordValue = "test-password"

		s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			return &api.InitiateRunResult{
				RunID:  "run-123",
				RunURL: "https://cloud.rwx.com/runs/run-123",
			}, nil
		}

		s.mockAPI.MockTaskKeyStatus = func(cfg api.TaskKeyStatusConfig) (api.TaskStatusResult, error) {
			return api.TaskStatusResult{
				Status: &api.TaskStatus{Result: api.TaskStatusSucceeded},
				TaskID: "task-456",
				Polling: api.PollingResult{
					Completed: true,
				},
			}, nil
		}

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

		cfg := cli.ImageBuildConfig{
			InitParameters: map[string]string{},
			MintFilePath:   "test.yml",
			TargetTaskKey:  "build-task",
			Tags:           []string{"latest"},
			Timeout:        1 * time.Second,
			OpenURL:        func(string) error { return nil },
		}

		_, err := s.service.ImageBuild(cfg)

		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to tag image as latest")
	})

	t.Run("returns unknown status error for unexpected status", func(t *testing.T) {
		s := setupTest(t)
		setupImageBuildTest(t, s)

		s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			return &api.InitiateRunResult{
				RunID:  "run-123",
				RunURL: "https://cloud.rwx.com/runs/run-123",
			}, nil
		}

		s.mockAPI.MockTaskKeyStatus = func(cfg api.TaskKeyStatusConfig) (api.TaskStatusResult, error) {
			return api.TaskStatusResult{
				Status: &api.TaskStatus{Result: "unknown-status"},
				Polling: api.PollingResult{
					Completed: true,
				},
			}, nil
		}

		cfg := cli.ImageBuildConfig{
			InitParameters: map[string]string{},
			MintFilePath:   "test.yml",
			TargetTaskKey:  "build-task",
			Timeout:        1 * time.Second,
			OpenURL:        func(string) error { return nil },
		}

		_, err := s.service.ImageBuild(cfg)

		require.Error(t, err)
		require.Equal(t, "build failed", err.Error())
	})

	t.Run("fails when status is nil", func(t *testing.T) {
		s := setupTest(t)
		setupImageBuildTest(t, s)

		s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			return &api.InitiateRunResult{
				RunID:  "run-123",
				RunURL: "https://cloud.rwx.com/runs/run-123",
			}, nil
		}

		s.mockAPI.MockTaskKeyStatus = func(cfg api.TaskKeyStatusConfig) (api.TaskStatusResult, error) {
			return api.TaskStatusResult{
				Status: nil,
				Polling: api.PollingResult{
					Completed: true,
				},
			}, nil
		}

		cfg := cli.ImageBuildConfig{
			InitParameters: map[string]string{},
			MintFilePath:   "test.yml",
			TargetTaskKey:  "build-task",
			Timeout:        1 * time.Second,
			OpenURL:        func(string) error { return nil },
		}

		_, err := s.service.ImageBuild(cfg)

		require.Error(t, err)
		require.Equal(t, "build failed", err.Error())
	})

	t.Run("fails when backoff instructions aren't included", func(t *testing.T) {
		s := setupTest(t)
		setupImageBuildTest(t, s)

		s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			return &api.InitiateRunResult{
				RunID:  "run-123",
				RunURL: "https://cloud.rwx.com/runs/run-123",
			}, nil
		}

		s.mockAPI.MockTaskKeyStatus = func(cfg api.TaskKeyStatusConfig) (api.TaskStatusResult, error) {
			return api.TaskStatusResult{
				Status: &api.TaskStatus{Result: "pending"},
				Polling: api.PollingResult{
					Completed: false,
					BackoffMs: nil,
				},
			}, nil
		}

		cfg := cli.ImageBuildConfig{
			InitParameters: map[string]string{},
			MintFilePath:   "test.yml",
			TargetTaskKey:  "build-task",
			Timeout:        1 * time.Second,
			OpenURL:        func(string) error { return nil },
		}

		_, err := s.service.ImageBuild(cfg)

		require.Error(t, err)
		require.Equal(t, "build failed", err.Error())
	})

	t.Run("skips pull when no-pull flag is set", func(t *testing.T) {
		s := setupTest(t)
		setupImageBuildTest(t, s)
		s.mockDocker.RegistryValue = "cloud.rwx.com"

		s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			return &api.InitiateRunResult{
				RunID:  "run-123",
				RunURL: "https://cloud.rwx.com/runs/run-123",
			}, nil
		}

		s.mockAPI.MockTaskKeyStatus = func(cfg api.TaskKeyStatusConfig) (api.TaskStatusResult, error) {
			return api.TaskStatusResult{
				Status:  &api.TaskStatus{Result: api.TaskStatusSucceeded},
				TaskID:  "task-456",
				Polling: api.PollingResult{Completed: true},
			}, nil
		}

		s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
			return &api.WhoamiResult{
				OrganizationSlug: "my-org",
			}, nil
		}

		pullCalled := false
		s.mockDocker.PullFunc = func(ctx context.Context, imageRef string, authConfig types.AuthConfig, quiet bool) error {
			pullCalled = true
			return nil
		}

		tagCalled := false
		s.mockDocker.TagFunc = func(ctx context.Context, source, target string) error {
			tagCalled = true
			return nil
		}

		cfg := cli.ImageBuildConfig{
			InitParameters: map[string]string{},
			MintFilePath:   "test.yml",
			TargetTaskKey:  "build-task",
			NoPull:         true,
			Timeout:        1 * time.Second,
			OpenURL:        func(string) error { return nil },
		}

		_, err := s.service.ImageBuild(cfg)

		require.NoError(t, err)
		require.False(t, pullCalled, "Pull should not be called when NoPull is true")
		require.False(t, tagCalled, "Tag should not be called when NoPull is true")
		require.Contains(t, s.mockStdout.String(), "Building image for build-task")
		require.Contains(t, s.mockStdout.String(), "Build succeeded!")
		require.Contains(t, s.mockStdout.String(), "Image available at: cloud.rwx.com/my-org:task-456")
		require.NotContains(t, s.mockStdout.String(), "Pulling image")
		require.NotContains(t, s.mockStdout.String(), "Image pulled successfully")
		require.NotContains(t, s.mockStdout.String(), "Tagging image")
	})

	t.Run("successful build with push-to references", func(t *testing.T) {
		s := setupTest(t)
		setupImageBuildTest(t, s)
		s.mockDocker.RegistryValue = "cloud.rwx.com"
		s.mockDocker.PasswordValue = "test-password"

		s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			return &api.InitiateRunResult{
				RunID:  "run-123",
				RunURL: "https://cloud.rwx.com/runs/run-123",
			}, nil
		}

		s.mockAPI.MockTaskKeyStatus = func(cfg api.TaskKeyStatusConfig) (api.TaskStatusResult, error) {
			return api.TaskStatusResult{
				Status:  &api.TaskStatus{Result: api.TaskStatusSucceeded},
				TaskID:  "task-456",
				Polling: api.PollingResult{Completed: true},
			}, nil
		}

		s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
			return &api.WhoamiResult{
				OrganizationSlug: "my-org",
			}, nil
		}

		s.mockDocker.PullFunc = func(ctx context.Context, imageRef string, authConfig types.AuthConfig, quiet bool) error {
			require.Equal(t, "cloud.rwx.com/my-org:task-456", imageRef)
			return nil
		}

		s.mockDocker.GetAuthConfigFunc = func(host string) (types.AuthConfig, error) {
			return types.AuthConfig{Username: "registry-user", Password: "registry-pass"}, nil
		}

		s.mockAPI.MockTaskIDStatus = func(cfg api.TaskIDStatusConfig) (api.TaskStatusResult, error) {
			return api.TaskStatusResult{
				Status:  &api.TaskStatus{Result: api.TaskStatusSucceeded},
				TaskID:  "task-456",
				Polling: api.PollingResult{Completed: true},
			}, nil
		}

		s.mockAPI.MockStartImagePush = func(cfg api.StartImagePushConfig) (api.StartImagePushResult, error) {
			require.Equal(t, "task-456", cfg.TaskID)
			require.Equal(t, "registry.com", cfg.Image.Registry)
			require.Equal(t, "myrepo", cfg.Image.Repository)
			require.ElementsMatch(t, []string{"latest", "v1.0"}, cfg.Image.Tags)
			return api.StartImagePushResult{PushID: "push-123", RunURL: "https://cloud.rwx.com/push/push-123"}, nil
		}

		s.mockAPI.MockImagePushStatus = func(pushID string) (api.ImagePushStatusResult, error) {
			require.Equal(t, "push-123", pushID)
			return api.ImagePushStatusResult{Status: "succeeded"}, nil
		}

		cfg := cli.ImageBuildConfig{
			InitParameters:   map[string]string{"key": "value"},
			MintFilePath:     "test.yml",
			NoCache:          false,
			TargetTaskKey:    "build-task",
			Tags:             []string{},
			PushToReferences: []string{"registry.com/myrepo:latest", "registry.com/myrepo:v1.0"},
			PushCompression:  "zstd",
			Timeout:          1 * time.Second,
			OpenURL:          func(string) error { return nil },
		}

		_, err := s.service.ImageBuild(cfg)

		require.NoError(t, err)
		require.Contains(t, s.mockStdout.String(), "Build succeeded!")
		require.Contains(t, s.mockStdout.String(), "Image pulled successfully!")
		require.Contains(t, s.mockStdout.String(), "Pushing image from task: task-456")
		require.Contains(t, s.mockStdout.String(), "Run URL: https://cloud.rwx.com/push/push-123")
		require.Contains(t, s.mockStdout.String(), "Image push succeeded!")
	})

	t.Run("fails when push fails", func(t *testing.T) {
		s := setupTest(t)
		setupImageBuildTest(t, s)
		s.mockDocker.RegistryValue = "cloud.rwx.com"
		s.mockDocker.PasswordValue = "test-password"

		s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			return &api.InitiateRunResult{
				RunID:  "run-123",
				RunURL: "https://cloud.rwx.com/runs/run-123",
			}, nil
		}

		s.mockAPI.MockTaskKeyStatus = func(cfg api.TaskKeyStatusConfig) (api.TaskStatusResult, error) {
			return api.TaskStatusResult{
				Status:  &api.TaskStatus{Result: api.TaskStatusSucceeded},
				TaskID:  "task-456",
				Polling: api.PollingResult{Completed: true},
			}, nil
		}

		s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
			return &api.WhoamiResult{
				OrganizationSlug: "my-org",
			}, nil
		}

		s.mockDocker.PullFunc = func(ctx context.Context, imageRef string, authConfig types.AuthConfig, quiet bool) error {
			return nil
		}

		s.mockDocker.GetAuthConfigFunc = func(host string) (types.AuthConfig, error) {
			return types.AuthConfig{}, fmt.Errorf("no credentials available")
		}

		cfg := cli.ImageBuildConfig{
			InitParameters:   map[string]string{},
			MintFilePath:     "test.yml",
			TargetTaskKey:    "build-task",
			PushToReferences: []string{"registry.com/myrepo:latest"},
			PushCompression:  "zstd",
			Timeout:          1 * time.Second,
			OpenURL:          func(string) error { return nil },
		}

		_, err := s.service.ImageBuild(cfg)

		require.Error(t, err)
		require.Contains(t, err.Error(), "no credentials available")
	})
}
