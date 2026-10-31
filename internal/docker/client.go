package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/docker/cli/cli/command"
	cliTypes "github.com/docker/cli/cli/config/types"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
	"github.com/moby/moby/pkg/jsonmessage"
	"github.com/rwx-cloud/rwx/internal/accesstoken"
	"github.com/rwx-cloud/rwx/internal/errors"
	"golang.org/x/term"
)

type Config struct {
	Registry           string
	AccessToken        string
	AccessTokenBackend accesstoken.Backend
}

func (c Config) Validate() error {
	if c.Registry == "" {
		return errors.New("missing registry")
	}
	return nil
}

type Client interface {
	GetAuthConfig(string) (cliTypes.AuthConfig, error)
	Pull(ctx context.Context, imageRef string, authConfig cliTypes.AuthConfig, quiet bool) error
	Load(context.Context, io.Reader, bool) error
	Tag(context.Context, string, string) error
	Registry() string
	Password() string
}

type realDockerCLI struct {
	*command.DockerCli
	registry string
	password string
}

func (r realDockerCLI) GetAuthConfig(registry string) (cliTypes.AuthConfig, error) {
	return r.ConfigFile().GetAuthConfig(registry)
}

func (r realDockerCLI) Pull(ctx context.Context, imageRef string, authConfig cliTypes.AuthConfig, quiet bool) error {
	encodedAuth, err := encodeAuthConfig(authConfig)
	if err != nil {
		return fmt.Errorf("unable to encode auth config: %w", err)
	}

	responseBody, err := r.Client().ImagePull(ctx, imageRef, image.PullOptions{
		RegistryAuth: encodedAuth,
	})
	if err != nil {
		return fmt.Errorf("unable to pull image: %w", err)
	}
	defer responseBody.Close()

	if quiet {
		// Consume the response body to complete the pull but discard output
		_, err = io.Copy(io.Discard, responseBody)
		return err
	}

	fd := os.Stdout.Fd()
	isTerminal := term.IsTerminal(int(fd))

	return jsonmessage.DisplayJSONMessagesStream(responseBody, r.Out(), fd, isTerminal, nil)
}

func (r realDockerCLI) Load(ctx context.Context, input io.Reader, quiet bool) error {
	response, err := r.Client().ImageLoad(ctx, input, client.ImageLoadWithQuiet(quiet))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	output := io.Writer(r.Out())
	if quiet {
		output = io.Discard
	}
	return jsonmessage.DisplayJSONMessagesStream(response.Body, output, os.Stdout.Fd(), false, nil)
}

func (r realDockerCLI) Tag(ctx context.Context, source, target string) error {
	err := r.Client().ImageTag(ctx, source, target)
	if err != nil {
		return fmt.Errorf("unable to tag image: %w", err)
	}
	return nil
}

func (r realDockerCLI) Registry() string {
	return r.registry
}

func (r realDockerCLI) Password() string {
	return r.password
}

func New(cfg Config) (Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	password, err := accesstoken.Get(cfg.AccessTokenBackend, cfg.AccessToken)
	if err != nil {
		return nil, errors.Wrap(err, "unable to retrieve access token")
	}

	cli, err := command.NewDockerCli()
	if err != nil {
		return nil, fmt.Errorf("unable to make a new docker CLI: %w", err)
	}

	if err := cli.Initialize(&flags.ClientOptions{}); err != nil {
		return nil, fmt.Errorf("unable to initialize the docker CLI: %w", err)
	}

	return realDockerCLI{
		DockerCli: cli,
		registry:  cfg.Registry,
		password:  password,
	}, nil
}

func encodeAuthConfig(authConfig cliTypes.AuthConfig) (string, error) {
	encodedJSON, err := json.Marshal(registry.AuthConfig{
		Username:      authConfig.Username,
		Password:      authConfig.Password,
		ServerAddress: authConfig.ServerAddress,
	})
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(encodedJSON), nil
}
