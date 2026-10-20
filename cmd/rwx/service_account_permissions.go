package main

import (
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/spf13/cobra"
)

func newServiceAccountPermissionsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "permissions",
		Short: "Manage service-account direct permissions",
		Long:  "Show or replace the entire direct-permission set. Inherited group permissions are unchanged. Credentials able to manage themselves or their groups are privileged.",
	}
	cmd.AddCommand(newServiceAccountPermissionsShowCommand(), newServiceAccountPermissionsReplaceCommand(), newServiceAccountPermissionsClearCommand())
	return cmd
}

func newServiceAccountPermissionsShowCommand() *cobra.Command {
	return newServiceAccountCommand("show", "Show direct permissions", func(cmd *cobra.Command, selector cli.ServiceAccountSelector) error {
		_, err := service.ShowServiceAccountPermissions(cli.ShowServiceAccountPermissionsConfig{ServiceAccountSelector: selector, Json: useJsonOutput()})
		return err
	})
}

func newServiceAccountPermissionsReplaceCommand() *cobra.Command {
	var file string
	var yes bool
	cmd := newServiceAccountCommand("replace", "Replace all direct permissions; inherited group permissions are unchanged", func(cmd *cobra.Command, selector cli.ServiceAccountSelector) error {
		_, err := service.ReplaceServiceAccountPermissions(cli.ReplaceServiceAccountPermissionsConfig{ServiceAccountSelector: selector, File: file, Yes: yes, Json: useJsonOutput()})
		return err
	})
	cmd.Flags().StringVar(&file, "file", "", "JSON file containing {\"permissions\":[{\"key\":\"run:read\",\"resource_id\":\"optional\"}]}; use - for stdin")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "confirm replacement without prompting")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func newServiceAccountPermissionsClearCommand() *cobra.Command {
	var yes bool
	cmd := newServiceAccountCommand("clear", "Clear all direct permissions; inherited group permissions are unchanged", func(cmd *cobra.Command, selector cli.ServiceAccountSelector) error {
		_, err := service.ClearServiceAccountPermissions(cli.ClearServiceAccountPermissionsConfig{ServiceAccountSelector: selector, Yes: yes, Json: useJsonOutput()})
		return err
	})
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "confirm clearing without prompting")
	return cmd
}

func init() {
	serviceAccountsCmd.AddCommand(newServiceAccountPermissionsCommand())
}
