package main

import (
	"os"
	"path/filepath"

	"github.com/rwx-cloud/rwx/cmd/rwx/config"
	"github.com/rwx-cloud/rwx/internal/accesstoken"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	internalconfig "github.com/rwx-cloud/rwx/internal/config"
	"github.com/rwx-cloud/rwx/internal/docker"
	"github.com/rwx-cloud/rwx/internal/docs"
	"github.com/rwx-cloud/rwx/internal/docstoken"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/rwx-cloud/rwx/internal/git"
	"github.com/rwx-cloud/rwx/internal/retry"
	"github.com/rwx-cloud/rwx/internal/ssh"
	"github.com/rwx-cloud/rwx/internal/telemetry"
	"github.com/rwx-cloud/rwx/internal/versions"
	"golang.org/x/term"

	"github.com/spf13/cobra"
)

var (
	AccessToken string
	Json        bool
	Output      string

	rwxHost            string
	docsHost           = "www.rwx.com"
	docsScheme         = "https"
	service            cli.Service
	accessTokenBackend accesstoken.Backend
	telem              *telemetry.Telemetry
	telemetryCollector *telemetry.Collector

	// rootCmd represents the main `rwx` command
	rootCmd = &cobra.Command{
		Use:           "rwx",
		Short:         "A CLI client from www.rwx.com",
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       config.Version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			fileBackend, err := internalconfig.NewFileBackend([]string{
				filepath.Join("~", ".config", "rwx"),
				filepath.Join("~", ".mint"),
			})
			if err != nil {
				return errors.Wrap(err, "unable to initialize config backend")
			}
			accessTokenBackend = accesstoken.NewFileBackend(fileBackend)
			docsTokenBackend := docstoken.NewFileBackend(fileBackend)
			versionsBackend := versions.NewFileBackend(fileBackend)
			skillVersionsBackend := versions.NewSkillFileBackend(fileBackend)

			c, err := api.NewClient(api.Config{
				AccessToken:          AccessToken,
				Host:                 rwxHost,
				AccessTokenBackend:   accessTokenBackend,
				VersionsBackend:      versionsBackend,
				SkillVersionsBackend: skillVersionsBackend,
			})
			if err != nil {
				return errors.Wrap(err, "unable to initialize API client")
			}

			dir, err := os.Getwd()
			if err != nil {
				return errors.Wrap(err, "unable to initialize CLI")
			}

			dockerCli, err := docker.New(docker.Config{
				Registry:           rwxHost,
				AccessToken:        AccessToken,
				AccessTokenBackend: accessTokenBackend,
			})
			if err != nil {
				return errors.Wrap(err, "unable to initialize Docker client")
			}

			service, err = cli.NewService(cli.Config{
				APIClient: c,
				SSHClient: new(ssh.Client),
				GitClient: &git.Client{
					Binary: "git",
					Dir:    dir,
				},
				DockerCLI:            dockerCli,
				DocsClient:           docs.Client{Host: docsHost, Scheme: docsScheme},
				DocsTokenBackend:     docsTokenBackend,
				AccessTokenBackend:   accessTokenBackend,
				VersionsBackend:      versionsBackend,
				SkillVersionsBackend: skillVersionsBackend,
				TelemetryCollector:   telemetryCollector,
				Stdin:                os.Stdin,
				Stdout:               os.Stdout,
				StdoutIsTTY:          term.IsTerminal(int(os.Stdout.Fd())),
				Stderr:               os.Stderr,
				StderrIsTTY:          term.IsTerminal(int(os.Stderr.Fd())),
			})
			if err != nil {
				return errors.Wrap(err, "unable to initialize CLI")
			}

			return nil
		},
	}
)

// initTelemetry builds the telemetry pipeline before Execute() runs so events
// are recorded for invocations that bypass PersistentPreRunE (e.g. unknown
// commands). Silent on failure — telemetry is best-effort and must never block
// the CLI.
func initTelemetry() {
	accessToken := AccessToken
	if accessToken == "$RWX_ACCESS_TOKEN" {
		accessToken = os.Getenv("RWX_ACCESS_TOKEN")
	}

	fileBackend, err := internalconfig.NewFileBackend([]string{
		filepath.Join("~", ".config", "rwx"),
		filepath.Join("~", ".mint"),
	})
	if err != nil {
		return
	}

	c, err := api.NewClient(api.Config{
		AccessToken:          accessToken,
		Host:                 rwxHost,
		AccessTokenBackend:   accesstoken.NewFileBackend(fileBackend),
		VersionsBackend:      versions.NewFileBackend(fileBackend),
		SkillVersionsBackend: versions.NewSkillFileBackend(fileBackend),
	})
	if err != nil {
		return
	}

	collector := telemetry.NewCollector()
	retryRT := retry.NewRoundTripper(c)
	statsRT := telemetry.NewStatsRoundTripper(retryRT)
	sender := telemetry.NewSender(collector, statsRT)
	telem = telemetry.New(collector, sender, statsRT)
	telemetryCollector = collector
}

func addRwxDirFlag(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&RwxDirectory, "dir", "d", "", "the directory your RWX configuration files are located in, typically `.rwx`. By default, the CLI traverses up until it finds a `.rwx` directory.")
}

func useJsonOutput() bool {
	return Output == "json" || Json
}

func init() {
	// A different host can only be set over the environment
	mintHostEnv := os.Getenv("MINT_HOST")
	rwxHostEnv := os.Getenv("RWX_HOST")

	if mintHostEnv == "" && rwxHostEnv == "" {
		rwxHost = "cloud.rwx.com"
	} else if mintHostEnv != "" {
		rwxHost = mintHostEnv
	} else {
		rwxHost = rwxHostEnv
	}

	if docsHostEnv := os.Getenv("RWX_DOCS_HOST"); docsHostEnv != "" {
		docsHost = docsHostEnv
		docsScheme = "http"
	}

	rootCmd.PersistentFlags().StringVar(&AccessToken, "access-token", "$RWX_ACCESS_TOKEN", "the access token for RWX")
	rootCmd.PersistentFlags().BoolVar(&Json, "json", false, "output json data to stdout")
	_ = rootCmd.PersistentFlags().MarkHidden("json")
	rootCmd.PersistentFlags().StringVar(&Output, "output", "text", "output format: text or json")

	// Define command groups for help output ordering
	rootCmd.AddGroup(&cobra.Group{ID: "execution", Title: "Execution:"})
	rootCmd.AddGroup(&cobra.Group{ID: "outputs", Title: "Outputs:"})
	rootCmd.AddGroup(&cobra.Group{ID: "api", Title: "API:"})
	rootCmd.AddGroup(&cobra.Group{ID: "definitions", Title: "Definitions:"})
	rootCmd.AddGroup(&cobra.Group{ID: "setup", Title: "Setup:"})

	// Set group IDs for built-in commands
	rootCmd.SetHelpCommandGroupID("setup")
	rootCmd.SetCompletionCommandGroupID("setup")

	// Add commands (GroupID is set in each command's definition)
	rootCmd.AddCommand(artifactsCmd)
	rootCmd.AddCommand(debugCmd)
	rootCmd.AddCommand(dispatchCmd)
	rootCmd.AddCommand(imageCmd)
	rootCmd.AddCommand(lintCmd)
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(lspCmd)
	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(packagesCmd)
	rootCmd.AddCommand(pushCmd)
	rootCmd.AddCommand(resolveCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(sandboxCmd)
	rootCmd.AddCommand(skillCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(vaultsCmd)
	rootCmd.AddCommand(docsCmd)
	rootCmd.AddCommand(resultsCmd)
	rootCmd.AddCommand(whoamiCmd)

	cobra.OnInitialize(func() {
		if AccessToken == "$RWX_ACCESS_TOKEN" {
			AccessToken = os.Getenv("RWX_ACCESS_TOKEN")
		}
	})
}
