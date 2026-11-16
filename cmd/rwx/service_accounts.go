package main

import (
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/spf13/cobra"
)

var serviceAccountsCmd = &cobra.Command{
	GroupID: "api",
	Use:     "service-accounts",
	Short:   "Manage service accounts",
}

func newServiceAccountsListCommand() *cobra.Command {
	var limit int
	var cursor string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List one page of service accounts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := service.ListServiceAccounts(cli.ListServiceAccountsConfig{Limit: limit, Cursor: cursor, Json: useJsonOutput()})
			return err
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "page size (max 100; server default applies when unset)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "cursor for fetching the next page")
	return cmd
}

func newServiceAccountsCreateCommand() *cobra.Command {
	var displayName, description string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a service account without issuing credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := cli.CreateServiceAccountConfig{DisplayName: displayName, Json: useJsonOutput()}
			if cmd.Flags().Changed("description") {
				cfg.Description = &description
			}
			_, err := service.CreateServiceAccount(cfg)
			return err
		},
	}
	cmd.Flags().StringVar(&displayName, "display-name", "", "display name for the new account")
	cmd.Flags().StringVar(&description, "description", "", "description for the new account")
	_ = cmd.MarkFlagRequired("display-name")
	return cmd
}

func newServiceAccountCommand(use, short string, operation func(*cobra.Command, cli.ServiceAccountSelector) error) *cobra.Command {
	var displayName string
	selector := func(args []string) cli.ServiceAccountSelector {
		cfg := cli.ServiceAccountSelector{DisplayName: displayName}
		if len(args) == 1 {
			cfg.ServiceAccount = args[0]
		}
		return cfg
	}
	cmd := &cobra.Command{
		Use:   use + " [ID_OR_NAME]",
		Short: short,
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.MaximumNArgs(1)(cmd, args); err != nil {
				return err
			}
			if cmd.Flags().Changed("display-name") && len(args) != 0 {
				return cli.ServiceAccountSelector{}.Validate()
			}
			return selector(args).Validate()
		},
		RunE: func(cmd *cobra.Command, args []string) error { return operation(cmd, selector(args)) },
	}
	cmd.Flags().StringVar(&displayName, "display-name", "", "look up by exact display name instead of ID or immutable name; rejects ambiguous matches")
	return cmd
}

func newServiceAccountsShowCommand() *cobra.Command {
	return newServiceAccountCommand("show", "Show a service account", func(cmd *cobra.Command, selector cli.ServiceAccountSelector) error {
		_, err := service.ShowServiceAccount(cli.ShowServiceAccountConfig{ServiceAccountSelector: selector, Json: useJsonOutput()})
		return err
	})
}

func newServiceAccountsUpdateCommand() *cobra.Command {
	var displayName, description string
	cmd := newServiceAccountCommand("update", "Update supplied service-account fields only", func(cmd *cobra.Command, selector cli.ServiceAccountSelector) error {
		cfg := cli.UpdateServiceAccountConfig{ServiceAccountSelector: selector, Json: useJsonOutput()}
		if cmd.Flags().Changed("new-display-name") {
			cfg.DisplayName = &displayName
		}
		if cmd.Flags().Changed("description") {
			cfg.Description = &description
		}
		_, err := service.UpdateServiceAccount(cfg)
		return err
	})
	cmd.Flags().StringVar(&displayName, "new-display-name", "", "new display name (immutable name does not change)")
	cmd.Flags().StringVar(&description, "description", "", "new description; an empty string clears it")
	return cmd
}

func newServiceAccountsDeactivateCommand() *cobra.Command {
	var yes bool
	cmd := newServiceAccountCommand("deactivate", "Deactivate a service account, revoke its tokens, and end its group memberships", func(cmd *cobra.Command, selector cli.ServiceAccountSelector) error {
		_, err := service.DeactivateServiceAccount(cli.DeactivateServiceAccountConfig{ServiceAccountSelector: selector, Yes: yes, Json: useJsonOutput()})
		return err
	})
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "confirm deactivation, token revocation, and ending group memberships without prompting")
	return cmd
}

func init() {
	serviceAccountsCmd.AddCommand(newServiceAccountsListCommand(), newServiceAccountsShowCommand(), newServiceAccountsCreateCommand(), newServiceAccountsUpdateCommand(), newServiceAccountsDeactivateCommand())
}
