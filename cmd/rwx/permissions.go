package main

import (
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/spf13/cobra"
)

var permissionsCmd = &cobra.Command{
	GroupID: "api",
	Use:     "permissions",
	Short:   "Discover available permissions",
}

var permissionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the live permission catalog and caller grantability",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		groupID, _ := cmd.Flags().GetString("group-id")
		serviceAccountID, _ := cmd.Flags().GetString("service-account-id")
		_, err := service.ListPermissions(cli.ListPermissionsConfig{
			GroupID: groupID, ServiceAccountID: serviceAccountID, Json: useJsonOutput(),
		})
		return err
	},
}

func init() {
	permissionsListCmd.Flags().String("group-id", "", "group ID for target-specific permission removability")
	permissionsListCmd.Flags().String("service-account-id", "", "service account ID for target-specific permission removability")
	permissionsListCmd.MarkFlagsMutuallyExclusive("group-id", "service-account-id")
	permissionsCmd.AddCommand(permissionsListCmd)
}
