package cli

import (
	"io"

	"github.com/rwx-cloud/rwx/internal/accesstoken"
	"github.com/rwx-cloud/rwx/internal/docker"
	"github.com/rwx-cloud/rwx/internal/docs"
	"github.com/rwx-cloud/rwx/internal/docstoken"
	"github.com/rwx-cloud/rwx/internal/errors"
	rwxssh "github.com/rwx-cloud/rwx/internal/ssh"
	"github.com/rwx-cloud/rwx/internal/telemetry"
	"github.com/rwx-cloud/rwx/internal/vcs"
	"github.com/rwx-cloud/rwx/internal/versions"
)

type Config struct {
	APIClient            APIClient
	SSHClient            SSHClient
	SSHTunnelManager     rwxssh.TunnelManager
	VCSClient            vcs.Client
	DockerCLI            docker.Client
	DocsClient           docs.Client
	DocsTokenBackend     docstoken.Backend
	AccessTokenBackend   accesstoken.Backend
	VersionsBackend      versions.Backend
	SkillVersionsBackend versions.Backend
	TelemetryCollector   *telemetry.Collector
	ChoicePicker         ChoicePicker
	Stdin                io.Reader
	Stdout               io.Writer
	StdoutIsTTY          bool
	Stderr               io.Writer
	StderrIsTTY          bool
}

func (c Config) Validate() error {
	if c.APIClient == nil {
		return errors.New("missing RWX client")
	}

	if c.SSHClient == nil {
		return errors.New("missing SSH client constructor")
	}

	if c.SSHTunnelManager == nil {
		return errors.New("missing SSH tunnel manager")
	}

	if c.VCSClient == nil {
		return errors.New("missing VCS client constructor")
	}

	if c.DockerCLI == nil {
		return errors.New("missing Docker client")
	}

	if c.Stdout == nil {
		return errors.New("missing Stdout")
	}

	if c.Stderr == nil {
		return errors.New("missing Stderr")
	}

	return nil
}
