package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/distribution/reference"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type ImagePushConfig struct {
	TaskID       string
	References   []reference.Named
	Compression  string
	JSON         bool
	Wait         bool
	OpenURL      func(url string) error
	PollInterval time.Duration
}

type ImagePushResult struct {
	PushID string `json:",omitempty"`
	RunURL string `json:",omitempty"`
	Status string `json:",omitempty"`
}

func NewImagePushConfig(taskID string, references []string, compression string, json bool, wait bool, openURL func(url string) error) (ImagePushConfig, error) {
	if taskID == "" {
		return ImagePushConfig{}, errors.New("a task ID must be provided")
	}

	if len(references) == 0 {
		return ImagePushConfig{}, errors.New("at least one OCI reference must be provided")
	}

	switch compression {
	case "zstd", "gzip", "none":
		// valid
	default:
		return ImagePushConfig{}, fmt.Errorf("unsupported compression %q: must be one of zstd, gzip, none", compression)
	}

	parsedReferences := make([]reference.Named, 0, len(references))
	for _, refStr := range references {
		ref, err := reference.ParseNormalizedNamed(refStr)
		if err != nil {
			return ImagePushConfig{}, errors.Wrapf(err, "invalid OCI reference: %s", refStr)
		}
		parsedReferences = append(parsedReferences, ref)
	}

	return ImagePushConfig{
		TaskID:       taskID,
		References:   parsedReferences,
		Compression:  compression,
		JSON:         json,
		Wait:         wait,
		OpenURL:      openURL,
		PollInterval: 1 * time.Second,
	}, nil
}

func (s Service) ImagePush(config ImagePushConfig) (pushResult *ImagePushResult, pushErr error) {
	start := time.Now()
	defer func() {
		s.recordTelemetry("image.push", map[string]any{
			"compression": config.Compression,
			"duration_ms": time.Since(start).Milliseconds(),
			"success":     pushErr == nil,
		})
	}()

	request := api.StartImagePushConfig{
		TaskID:      config.TaskID,
		Image:       api.StartImagePushConfigImage{},
		Credentials: api.StartImagePushConfigCredentials{},
		Compression: config.Compression,
	}

	for _, ref := range config.References {
		registry := reference.Domain(ref)
		if registry == "docker.io" {
			registry = "registry-1.docker.io"
		}

		repository := reference.Path(ref)

		tag := "latest"
		if tagged, ok := ref.(reference.Tagged); ok {
			tag = tagged.Tag()
		}

		if request.Image.Registry == "" {
			request.Image.Registry = registry
		} else if request.Image.Registry != registry {
			return nil, fmt.Errorf("all image references must have the same registry: %v != %v", request.Image.Registry, registry)
		}

		if request.Image.Repository == "" {
			request.Image.Repository = repository
		} else if request.Image.Repository != repository {
			return nil, fmt.Errorf("all image references must have the same repository: %v != %v", request.Image.Repository, repository)
		}

		request.Image.Tags = append(request.Image.Tags, tag)
	}

	request.Credentials.Username = os.Getenv("RWX_PUSH_USERNAME")
	request.Credentials.Password = os.Getenv("RWX_PUSH_PASSWORD")

	if request.Credentials.Username == "" && request.Credentials.Password != "" {
		return nil, fmt.Errorf("RWX_PUSH_USERNAME must be set if RWX_PUSH_PASSWORD is set")
	} else if request.Credentials.Username != "" && request.Credentials.Password == "" {
		return nil, fmt.Errorf("RWX_PUSH_PASSWORD must be set if RWX_PUSH_USERNAME is set")
	} else if request.Credentials.Username == "" && request.Credentials.Password == "" {
		credentialsHost := request.Image.Registry
		if credentialsHost == "registry-1.docker.io" {
			credentialsHost = "index.docker.io"
		}

		credentials, err := s.DockerCLI.GetAuthConfig(credentialsHost)
		if err != nil {
			return nil, fmt.Errorf("unable to get credentials for registry %q from docker: %w", request.Image.Registry, err)
		}
		if credentials.Username == "" || credentials.Password == "" {
			return nil, fmt.Errorf("no credentials found for registry %q in docker config", request.Image.Registry)
		}

		request.Credentials.Username = credentials.Username
		request.Credentials.Password = credentials.Password
	}

	stopStartSpinner := func() {}
	if !config.JSON {
		fmt.Fprintf(s.Stdout, "Pushing image from task: %s\n", request.TaskID)
		for _, tag := range request.Image.Tags {
			fmt.Fprintf(s.Stdout, "%s/%s:%s\n", request.Image.Registry, request.Image.Repository, tag)
		}

		stopStartSpinner = Spin(
			"Starting...",
			s.StderrIsTTY,
			s.Stderr,
		)
	}

	taskStatusTimeout, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	succeeded := false
	for !succeeded {
		select {
		case <-taskStatusTimeout.Done():
			stopStartSpinner()
			return nil, fmt.Errorf("timed out waiting for task %s to complete", config.TaskID)
		default:
		}

		result, err := s.APIClient.TaskIDStatus(api.TaskIDStatusConfig{TaskID: config.TaskID})
		if err != nil {
			stopStartSpinner()
			return nil, fmt.Errorf("failed to get task status: %w", err)
		}

		if result.Polling.Completed {
			if taskStatus := result.GetTaskStatus(); taskStatus != nil && taskStatus.Result == api.TaskStatusSucceeded {
				succeeded = true
			} else {
				stopStartSpinner()
				return nil, fmt.Errorf("task failed")
			}
		} else {
			time.Sleep(time.Duration(*result.Polling.BackoffMs) * time.Millisecond)
		}
	}

	result, err := s.APIClient.StartImagePush(request)
	stopStartSpinner()
	if err != nil {
		return nil, err
	}

	if !config.JSON {
		fmt.Fprintf(s.Stdout, "Starting RWX run to push image: %s\n", result.RunURL)
	}

	if err := config.OpenURL(result.RunURL); err != nil {
		if err.Error() != "" {
			fmt.Fprintf(s.Stderr, "Warning: unable to open the run in your browser.\n")
		}
	}

	if !config.Wait {
		output := &ImagePushResult{PushID: result.PushID, RunURL: result.RunURL}
		if config.JSON {
			if err := json.NewEncoder(s.Stdout).Encode(output); err != nil {
				return nil, fmt.Errorf("unable to encode output: %w", err)
			}
		} else {
			fmt.Fprintln(s.Stdout, "Your image is being pushed. This may take some time for large images.")
			fmt.Fprintln(s.Stdout)
		}
		return output, nil
	}

	stopWaitingSpinner := func() {}
	if !config.JSON {
		stopWaitingSpinner = Spin(
			"Waiting for image push to finish...",
			s.StderrIsTTY,
			s.Stderr,
		)
	}

	pollInterval := config.PollInterval
	if pollInterval == 0*time.Second {
		pollInterval = 1 * time.Millisecond
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	var finalPushResult api.ImagePushStatusResult
statusloop:
	for range ticker.C {
		pushStatus, err := s.APIClient.ImagePushStatus(result.PushID)
		if err != nil {
			stopWaitingSpinner()
			return nil, fmt.Errorf("unable to get image push status: %w", err)
		}

		switch pushStatus.Status {
		case "succeeded", "failed":
			finalPushResult = pushStatus
			stopWaitingSpinner()
			break statusloop
		case "in_progress", "waiting":
			// continue waiting
		default:
			stopWaitingSpinner()
			return nil, fmt.Errorf("unknown image push status: %q", pushStatus.Status)
		}
	}

	output := &ImagePushResult{PushID: result.PushID, RunURL: result.RunURL, Status: finalPushResult.Status}

	switch finalPushResult.Status {
	case "succeeded":
		if config.JSON {
			if err := json.NewEncoder(s.Stdout).Encode(output); err != nil {
				return nil, fmt.Errorf("unable to encode output: %w", err)
			}
		} else {
			fmt.Fprintf(s.Stdout, "Image push succeeded!\n")
		}
		return output, nil
	case "failed":
		if config.JSON {
			if err := json.NewEncoder(s.Stdout).Encode(output); err != nil {
				return nil, fmt.Errorf("unable to encode output: %w", err)
			}
		}
		return output, fmt.Errorf("image push failed, inspect the run at %q to see why", result.RunURL)
	default:
		return nil, fmt.Errorf("unknown image push status: %q", finalPushResult.Status)
	}
}
