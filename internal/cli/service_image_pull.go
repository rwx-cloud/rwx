package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	cliTypes "github.com/docker/cli/cli/config/types"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/rwx-cloud/rwx/internal/imagearchive"
)

type ImagePullConfig struct {
	TaskID     string
	Tags       []string
	Timeout    time.Duration
	OutputJSON bool
}

func (c ImagePullConfig) Validate() error {
	if c.TaskID == "" {
		return errors.New("task ID must be provided")
	}
	return nil
}

type ImagePullResult struct {
	ImageRef string   `json:",omitempty"`
	Tags     []string `json:",omitempty"`
}

func (s Service) ImagePull(config ImagePullConfig) (pullResult *ImagePullResult, pullErr error) {
	start := time.Now()
	defer func() {
		s.recordTelemetry("image.pull", map[string]any{
			"duration_ms": time.Since(start).Milliseconds(),
			"success":     pullErr == nil,
		})
	}()

	if err := config.Validate(); err != nil {
		return nil, err
	}

	whoamiResult, err := s.APIClient.Whoami()
	if err != nil {
		return nil, fmt.Errorf("failed to get organization info: %w\nTry running `rwx login` again", err)
	}

	registry := s.DockerCLI.Registry()
	imageRef := fmt.Sprintf("%s/%s:%s", registry, whoamiResult.OrganizationSlug, config.TaskID)

	if !config.OutputJSON {
		fmt.Fprintf(s.Stdout, "Pulling image: %s\n", imageRef)
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	authConfig := cliTypes.AuthConfig{
		Username:      whoamiResult.OrganizationSlug,
		Password:      s.DockerCLI.Password(),
		ServerAddress: registry,
	}

	imageConfig, err := s.APIClient.ImagePullConfiguration(ctx, config.TaskID)
	if err != nil && !errors.Is(err, api.ErrNotFound) {
		return nil, fmt.Errorf("failed to get image configuration: %w", err)
	}
	local := false
	if imageConfig != nil {
		for _, layer := range imageConfig.Layers {
			if layer.Kind == "whiteout" && layer.SHA256 == "" {
				local = true
			}
		}
	}
	if local {
		err = s.loadTaskImage(ctx, imageConfig, imageRef, config.OutputJSON)
	} else {
		err = s.DockerCLI.Pull(ctx, imageRef, authConfig, config.OutputJSON)
	}
	if err != nil {
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

	result := &ImagePullResult{
		ImageRef: imageRef,
		Tags:     config.Tags,
	}

	if config.OutputJSON {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, fmt.Errorf("unable to encode output: %w", err)
		}
	}

	return result, nil
}

func (s Service) loadTaskImage(ctx context.Context, config *api.ImageConfiguration, imageRef string, quiet bool) error {
	f, err := os.CreateTemp("", "rwx-image-*.tar")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := imagearchive.Write(ctx, f, config, imageRef); err != nil {
		return err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	return s.DockerCLI.Load(ctx, f, quiet)
}
