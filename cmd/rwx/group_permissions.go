package main

import (
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/spf13/cobra"
)

func newGroupPermissionsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "permissions",
		Short: "Read or replace group direct permissions",
		Long:  "Read or replace the complete direct-permission set for a group. Inherited permissions remain unchanged. The server validates replacements atomically and remains authoritative.",
	}
	show := &cobra.Command{
		Use:   "show GROUP",
		Short: "Show direct permissions (not inherited permissions)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := service.ShowGroupPermissions(cli.ShowGroupPermissionsConfig{Group: args[0], Json: useJsonOutput()})
			return err
		},
	}
	var file string
	var replaceYes, clearYes bool
	replace := &cobra.Command{
		Use:   "replace GROUP",
		Short: "Replace the complete direct-permission set; inherited permissions unchanged",
		Long: `Replace the complete direct-permission set; inherited permissions remain unchanged.
Use --file PATH (or --file - for stdin) with JSON such as:
{"permissions":[{"key":"run:read"},{"key":"vault:manage","resource_id":"vault-id"}]}
An empty permissions array explicitly clears direct permissions. The server applies all changes or none.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := service.ReplaceGroupPermissions(cli.ReplaceGroupPermissionsConfig{Group: args[0], File: file, Yes: replaceYes, Json: useJsonOutput()})
			return err
		},
	}
	replace.Flags().StringVar(&file, "file", "", "replacement JSON file, or - for stdin")
	_ = replace.MarkFlagRequired("file")
	replace.Flags().BoolVarP(&replaceYes, "yes", "y", false, "skip confirmation prompt")
	clear := &cobra.Command{
		Use:   "clear GROUP",
		Short: "Clear all direct permissions; inherited permissions unchanged",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := service.ClearGroupPermissions(cli.ClearGroupPermissionsConfig{Group: args[0], Yes: clearYes, Json: useJsonOutput()})
			return err
		},
	}
	clear.Flags().BoolVarP(&clearYes, "yes", "y", false, "skip confirmation prompt")
	cmd.AddCommand(show, replace, clear)
	return cmd
}
