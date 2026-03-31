package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	cliTypes "github.com/docker/cli/cli/config/types"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type ImageBuildConfig struct {
	InitParameters   map[string]string
	RwxDirectory     string
	MintFilePath     string
	NoCache          bool
	NoPull           bool
	TargetTaskKey    string
	Tags             []string
	PushToReferences []string
	PushCompression  string
	Timeout          time.Duration
	OpenURL          func(string) error
	OutputJSON       bool
}

func (c ImageBuildConfig) Validate() error {
	if c.MintFilePath == "" {
		return errors.New("the path to a run definition must be provided")
	}
	if c.TargetTaskKey == "" {
		return errors.New("a target task key must be provided")
	}
	return nil
}

type ImageBuildResult struct {
	RunURL   string           `json:",omitempty"`
	ImageRef string           `json:",omitempty"`
	TaskID   string           `json:",omitempty"`
	Tags     []string         `json:",omitempty"`
	Push     *ImagePushResult `json:",omitempty"`
}

func (s Service) ImageBuild(config ImageBuildConfig) (buildResult *ImageBuildResult, buildErr error) {
	start := time.Now()
	defer func() {
		s.recordTelemetry("image.build", map[string]any{
			"duration_ms": time.Since(start).Milliseconds(),
			"success":     buildErr == nil,
		})
	}()

	if err := config.Validate(); err != nil {
		return nil, err
	}

	runResult, err := s.InitiateRun(InitiateRunConfig{
		InitParameters: config.InitParameters,
		RwxDirectory:   config.RwxDirectory,
		MintFilePath:   config.MintFilePath,
		NoCache:        config.NoCache,
		TargetedTasks:  []string{config.TargetTaskKey},
		Patchable:      true,
	})
	if err != nil {
		return nil, err
	}

	result := &ImageBuildResult{
		RunURL: runResult.RunURL,
	}

	if !config.OutputJSON {
		fmt.Fprintf(s.Stdout, "Building image for %s\n", config.TargetTaskKey)
		fmt.Fprintf(s.Stdout, "Run URL: %s\n\n", runResult.RunURL)
	}

	if err := config.OpenURL(runResult.RunURL); err != nil {
		return nil, fmt.Errorf("failed to open URL: %w", err)
	}

	stopSpinner := func() {}
	if !config.OutputJSON {
		stopSpinner = Spin(
			"Polling for build completion...",
			s.StderrIsTTY,
			s.Stderr,
		)
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	var taskID string
	succeeded := false
	for !succeeded {
		select {
		case <-ctx.Done():
			stopSpinner()
			return nil, fmt.Errorf("timeout waiting for build to complete after %s\n\nThe build may still be running. Check the status at: %s", config.Timeout, runResult.RunURL)
		default:
		}

		statusResult, err := s.APIClient.TaskKeyStatus(api.TaskKeyStatusConfig{
			RunID:   runResult.RunID,
			TaskKey: config.TargetTaskKey,
		})
		if err != nil {
			stopSpinner()
			return nil, fmt.Errorf("failed to get build status: %w", err)
		}

		if statusResult.Polling.Completed {
			if taskStatus := statusResult.GetTaskStatus(); taskStatus != nil && taskStatus.Result == api.TaskStatusSucceeded {
				taskID = statusResult.TaskID
				stopSpinner()
				if !config.OutputJSON {
					fmt.Fprintf(s.Stdout, "\nBuild succeeded!\n\n")
				}
				succeeded = true
			} else {
				stopSpinner()
				return nil, fmt.Errorf("build failed")
			}
		} else {
			if statusResult.Polling.BackoffMs == nil {
				stopSpinner()
				return nil, fmt.Errorf("build failed")
			}
			time.Sleep(time.Duration(*statusResult.Polling.BackoffMs) * time.Millisecond)
		}
	}

	result.TaskID = taskID

	whoamiResult, err := s.APIClient.Whoami()
	if err != nil {
		return nil, fmt.Errorf("failed to get organization info: %w\nTry running `rwx login` again", err)
	}

	registry := s.DockerCLI.Registry()
	imageRef := fmt.Sprintf("%s/%s:%s", registry, whoamiResult.OrganizationSlug, taskID)
	result.ImageRef = imageRef

	if config.NoPull {
		if !config.OutputJSON {
			fmt.Fprintf(s.Stdout, "Image available at: %s\n", imageRef)
		} else {
			if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
				return nil, fmt.Errorf("unable to encode output: %w", err)
			}
		}
		return result, nil
	}

	if !config.OutputJSON {
		fmt.Fprintf(s.Stdout, "Pulling image: %s\n", imageRef)
	}

	authConfig := cliTypes.AuthConfig{
		Username:      whoamiResult.OrganizationSlug,
		Password:      s.DockerCLI.Password(),
		ServerAddress: registry,
	}

	if err := s.DockerCLI.Pull(ctx, imageRef, authConfig, config.OutputJSON); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("timeout while pulling image after %s\n\nThe image may still be available at: %s", config.Timeout, imageRef)
		}
		return nil, fmt.Errorf("failed to pull image: %w", err)
	}

	if !config.OutputJSON {
		fmt.Fprintf(s.Stdout, "\nImage pulled successfully!\n")
	}

	for _, tag := range config.Tags {
		if !config.OutputJSON {
			fmt.Fprintf(s.Stdout, "Tagging image as: %s\n", tag)
		}

		if err := s.DockerCLI.Tag(ctx, imageRef, tag); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, fmt.Errorf("timeout while tagging image after %s", config.Timeout)
			}
			return nil, fmt.Errorf("failed to tag image as %s: %w", tag, err)
		}
	}
	result.Tags = config.Tags

	if len(config.PushToReferences) > 0 {
		if !config.OutputJSON {
			fmt.Fprintf(s.Stdout, "\n")
		}

		pushConfig, err := NewImagePushConfig(
			taskID,
			config.PushToReferences,
			config.PushCompression,
			config.OutputJSON,
			true,
			func(url string) error {
				if !config.OutputJSON {
					fmt.Fprintf(s.Stdout, "Run URL: %s\n", url)
				}
				return nil
			},
		)
		if err != nil {
			return nil, err
		}

		pushResult, err := s.ImagePush(pushConfig)
		if err != nil {
			return nil, err
		}
		result.Push = pushResult
	}

	if config.OutputJSON {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, fmt.Errorf("unable to encode output: %w", err)
		}
	}

	return result, nil
}
