package main

import (
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/spf13/cobra"
)

var appsCmd = &cobra.Command{
	GroupID: "api",
	Use:     "apps",
	Short:   "Manage preview apps",
}

var appsDisableCmd = &cobra.Command{
	Use:   "disable ENDPOINT_OR_URL",
	Short: "Disable a preview app endpoint",
	Long:  "Disable a preview app by endpoint or URL and queue running instances for shutdown.",
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

var appsShareCmd = &cobra.Command{
	Use:   "share ENDPOINT_OR_URL",
	Short: "Create a share link for a private preview app",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		description, _ := cmd.Flags().GetString("description")
		expiresIn, _ := cmd.Flags().GetString("expires-in")
		_, err := service.ShareApp(cli.ShareAppConfig{
			Endpoint: args[0], Description: description, ExpiresIn: expiresIn, Json: useJsonOutput(),
		})
		return err
	},
}

func init() {
	appsShareCmd.Flags().String("description", "", "optional description for the share link")
	appsShareCmd.Flags().String("expires-in", "7d", "how long the link grants access (positive duration, e.g. 1h, 1d, 7d)")
	appsCmd.AddCommand(appsDisableCmd, appsEnableCmd, appsShareCmd)
}
