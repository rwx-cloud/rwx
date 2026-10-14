package main

import (
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/spf13/cobra"
)

var billingCmd = &cobra.Command{
	GroupID: "api",
	Use:     "billing",
	Short:   "View billing usage",
}

var billingUsageCmd = newBillingUsageCommand()

func newBillingUsageCommand() *cobra.Command {
	var cfg cli.BillingUsageConfig
	cmd := &cobra.Command{
		Use:     "usage",
		Short:   "Show monthly billing usage",
		Long:    "Show billing usage for the authenticated organization.\n\nOmit --month to use the current billing month in UTC. Use --month YYYY-MM to select a month. Future months are rejected.\n\nAmounts and quantities retain their decimal precision. Available days can contain estimates.",
		Example: "  rwx billing usage\n  rwx billing usage --month 2026-09\n  rwx billing usage --month 2026-09 --json",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.NoArgs(cmd, args); err != nil {
				return err
			}
			if cmd.Flags().Changed("month") && cfg.Month == "" {
				return errors.New("month must be in YYYY-MM format")
			}
			return cfg.Validate()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Json = useJsonOutput()
			_, err := service.BillingUsage(cfg)
			return err
		},
	}
	cmd.Flags().StringVar(&cfg.Month, "month", "", "billing month in YYYY-MM format (defaults to the API's current month)")
	return cmd
}

func init() {
	billingCmd.AddCommand(billingUsageCmd)
}
