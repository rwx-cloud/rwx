package main

import (
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/spf13/cobra"
)

func newGroupRelationshipsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "relationships",
		Short: "Manage parent/child group permission inheritance",
		Long:  "Inspect or change one parent/child pair. The child inherits the parent's permissions, not the reverse. Reads require org:access; changes require group:manage on the parent.",
	}
	for _, action := range []struct {
		name  string
		short string
		run   func(cli.Service, cli.GroupRelationshipConfig) (*cli.GroupRelationshipResult, error)
	}{
		{"show", "Show whether the child inherits the parent's permissions", cli.Service.ShowGroupRelationship},
		{"add", "Make the child inherit the parent's permissions", cli.Service.AddGroupRelationship},
		{"remove", "Remove the relationship without deleting either group", cli.Service.RemoveGroupRelationship},
	} {
		var cfg cli.GroupRelationshipConfig
		sub := &cobra.Command{
			Use:   action.name,
			Short: action.short,
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg.Json = useJsonOutput()
				_, err := action.run(service, cfg)
				return err
			},
		}
		sub.Flags().StringVar(&cfg.Parent, "parent", "", "parent group ID or exact name (permissions source)")
		sub.Flags().StringVar(&cfg.Child, "child", "", "child group ID or exact name (inherits permissions)")
		_ = sub.MarkFlagRequired("parent")
		_ = sub.MarkFlagRequired("child")
		if action.name == "remove" {
			sub.Flags().BoolVarP(&cfg.Yes, "yes", "y", false, "skip confirmation prompt")
		}
		cmd.AddCommand(sub)
	}
	return cmd
}
