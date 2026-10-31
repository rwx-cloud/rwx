package mocks

import (
	"context"
	"io"

	cliTypes "github.com/docker/cli/cli/config/types"
)

type DockerClient struct {
	LoadFunc          func(context.Context, io.Reader, bool) error
	GetAuthConfigFunc func(string) (cliTypes.AuthConfig, error)
	PullFunc          func(context.Context, string, cliTypes.AuthConfig, bool) error
	TagFunc           func(context.Context, string, string) error
	RegistryValue     string
	PasswordValue     string
}

func (m *DockerClient) GetAuthConfig(registry string) (cliTypes.AuthConfig, error) {
	if m.GetAuthConfigFunc != nil {
		return m.GetAuthConfigFunc(registry)
	}
	return cliTypes.AuthConfig{}, nil
}

func (m *DockerClient) Pull(ctx context.Context, imageRef string, authConfig cliTypes.AuthConfig, quiet bool) error {
	if m.PullFunc != nil {
		return m.PullFunc(ctx, imageRef, authConfig, quiet)
	}
	return nil
}

func (m *DockerClient) Load(ctx context.Context, input io.Reader, quiet bool) error {
	if m.LoadFunc != nil {
		return m.LoadFunc(ctx, input, quiet)
	}
	return nil
}

func (m *DockerClient) Tag(ctx context.Context, source, target string) error {
	if m.TagFunc != nil {
		return m.TagFunc(ctx, source, target)
	}
	return nil
}

func (m *DockerClient) Registry() string {
	return m.RegistryValue
}

func (m *DockerClient) Password() string {
	return m.PasswordValue
}
