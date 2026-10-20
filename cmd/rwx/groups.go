package main

import (
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/spf13/cobra"
)

var groupsCmd = &cobra.Command{
	GroupID: "api",
	Use:     "groups",
	Short:   "Manage groups",
}

func newGroupsListCommand() *cobra.Command {
	var cfg cli.ListGroupsConfig
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List groups",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Json = useJsonOutput()
			_, err := service.ListGroups(cfg)
			return err
		},
	}
	cmd.Flags().IntVar(&cfg.Limit, "limit", 0, "page size (max 100; server default applies when unset)")
	cmd.Flags().StringVar(&cfg.Cursor, "cursor", "", "cursor for fetching the next page")
	return cmd
}

func newGroupsShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show NAME_OR_ID",
		Short: "Show a group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := service.ShowGroup(cli.ShowGroupConfig{Group: args[0], Json: useJsonOutput()})
			return err
		},
	}
}

func newGroupsCreateCommand() *cobra.Command {
	var name, description string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a group",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := cli.CreateGroupConfig{Name: name, Json: useJsonOutput()}
			if cmd.Flags().Changed("description") {
				cfg.Description = &description
			}
			_, err := service.CreateGroup(cfg)
			return err
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "group name")
	cmd.Flags().StringVar(&description, "description", "", "group description")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newGroupsUpdateCommand() *cobra.Command {
	var name, description string
	cmd := &cobra.Command{
		Use:   "update NAME_OR_ID",
		Short: "Update group metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := cli.UpdateGroupConfig{Group: args[0], Json: useJsonOutput()}
			if cmd.Flags().Changed("name") {
				cfg.Name = &name
			}
			if cmd.Flags().Changed("description") {
				cfg.Description = &description
			}
			_, err := service.UpdateGroup(cfg)
			return err
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "group name")
	cmd.Flags().StringVar(&description, "description", "", "group description (empty clears it)")
	return cmd
}

func newGroupsDeleteCommand() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete NAME_OR_ID",
		Short: "Delete a group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := service.DeleteGroup(cli.DeleteGroupConfig{Group: args[0], Yes: yes, Json: useJsonOutput()})
			return err
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")
	return cmd
}

func init() {
	groupsCmd.AddCommand(newGroupsListCommand(), newGroupsShowCommand(), newGroupsCreateCommand(), newGroupsUpdateCommand(), newGroupsDeleteCommand())
}
