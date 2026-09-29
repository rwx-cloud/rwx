package main

import (
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/spf13/cobra"
)

var cronsCmd = &cobra.Command{
	GroupID: "api",
	Use:     "crons",
	Short:   "Manage crons",
	Long:    "List crons and manage their schedules.\n\nChoose a cron by its key. If more than one matches, we'll ask you to choose in a terminal. For scripts, narrow it down with --repository, --branch, or --file, or use --id instead of a key. Archived crons can only be selected by ID.",
}

var cronsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List crons",
	Long:  "List your crons and their schedules. Archived crons are not included.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := service.ListCrons(cli.ListCronsConfig{Json: useJsonOutput()})
		return err
	},
}

var cronsShowCmd = newCronCommand("show [KEY]", "Show a cron's schedule and status",
	func(cfg cli.CronConfig) (*cli.CronResult, error) {
		return service.ShowCron(cfg)
	},
)

var cronsPauseCmd = newCronCommand("pause [KEY]", "Pause a cron",
	func(cfg cli.CronConfig) (*cli.CronResult, error) {
		return service.PauseCron(cfg)
	},
)

var cronsResumeCmd = newCronCommand("resume [KEY]", "Resume a cron",
	func(cfg cli.CronConfig) (*cli.CronResult, error) {
		return service.ResumeCron(cfg)
	},
)

var cronsSnoozeCmd = newCronSnoozeCommand()

func newCronSnoozeCommand() *cobra.Command {
	var until string
	cmd := newCronCommand("snooze [KEY] --until TIMESTAMP", "Snooze a cron until a specific time",
		func(cfg cli.CronConfig) (*cli.CronResult, error) {
			return service.SnoozeCron(cli.SnoozeCronConfig{CronConfig: cfg, Until: until})
		},
	)
	cmd.Long = "Skip scheduled runs until the given time.\n\nInclude a time zone, for example 2027-01-02T09:00:00-05:00 or 2027-01-02T14:00:00Z. Seconds are ignored, and the chosen minute must be more than two minutes ahead."
	cmd.Flags().StringVar(&until, "until", "", "when to resume (e.g. 2027-01-02T09:00:00-05:00)")
	_ = cmd.MarkFlagRequired("until")
	return cmd
}

func newCronCommand(use, short string, operation func(cli.CronConfig) (*cli.CronResult, error)) *cobra.Command {
	var target cli.CronConfig
	config := func(args []string) cli.CronConfig {
		cfg := target
		if len(args) == 1 {
			cfg.Key = args[0]
		}
		cfg.Json = useJsonOutput()
		return cfg
	}
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.MaximumNArgs(1)(cmd, args); err != nil {
				return err
			}
			return config(args).Validate()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := operation(config(args))
			return err
		},
	}
	cmd.Flags().StringVar(&target.ID, "id", "", "select a cron by ID instead of key")
	cmd.Flags().StringVar(&target.Repository, "repository", "", "match the repository shown by 'rwx crons list'")
	cmd.Flags().StringVar(&target.Branch, "branch", "", "match a branch")
	cmd.Flags().StringVar(&target.File, "file", "", "match a run definition path (e.g. .rwx/ci.yml)")
	return cmd
}

func init() {
	cronsCmd.AddCommand(cronsListCmd, cronsShowCmd, cronsPauseCmd, cronsResumeCmd, cronsSnoozeCmd)
}
