package main

import (
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/spf13/cobra"
)

var appsCmd = &cobra.Command{
	GroupID: "api",
	Use:     "apps",
	Short:   "Manage preview apps",
	Long:    "Enable or disable preview apps by endpoint or URL.",
}

var appsDisableCmd = &cobra.Command{
	Use:   "disable ENDPOINT_OR_URL",
	Short: "Disable a preview app endpoint",
	Long:  "Disable a preview app by endpoint or URL, including all versions, and queue running instances for shutdown.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := service.DisableApp(cli.AppConfig{Endpoint: args[0], Json: useJsonOutput()})
		return err
	},
}

var appsEnableCmd = &cobra.Command{
	Use:   "enable ENDPOINT_OR_URL",
	Short: "Enable a preview app endpoint",
	Long:  "Enable a preview app by endpoint or URL. Stopped apps cold-start on a subsequent visit; permanently expired versions remain expired. Enabling an already-enabled endpoint does not interrupt a running app.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := service.EnableApp(cli.AppConfig{Endpoint: args[0], Json: useJsonOutput()})
		return err
	},
}

func init() {
	appsCmd.AddCommand(appsDisableCmd, appsEnableCmd)
}
