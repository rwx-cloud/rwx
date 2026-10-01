package main

import (
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/spf13/cobra"
)

var vaultsCmd = &cobra.Command{
	GroupID: "api",
	Short:   "Manage vaults, secrets, and vars",
	Use:     "vaults",
}

var vaultsListCmd = &cobra.Command{
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := service.ListVaults(cli.ListVaultsConfig{Json: useJsonOutput()})
		return err
	},
	Short: "List vaults",
	Use:   "list",
}

// --- vaults create ---

var (
	createVaultName      string
	createVaultUnlocked  bool
	createVaultRepoPerms []string

	vaultsCreateCmd = &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			useJson := useJsonOutput()
			_, err := service.CreateVault(cli.CreateVaultConfig{
				Name:                  createVaultName,
				Unlocked:              createVaultUnlocked,
				RepositoryPermissions: createVaultRepoPerms,
				Json:                  useJson,
			})
			return err
		},
		Short: "Create a new vault",
		Use:   "create [flags]",
	}
)

var vaultsShowCmd = &cobra.Command{
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := service.ShowVault(cli.ShowVaultConfig{
			Vault: args[0],
			Json:  useJsonOutput(),
		})
		return err
	},
	Short: "Show a vault",
	Use:   "show NAME_OR_ID",
}

var (
	updateVaultName     string
	updateVaultUnlocked bool

	vaultsUpdateCmd = &cobra.Command{
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := service.UpdateVault(cli.UpdateVaultConfig{
				Vault:       args[0],
				Name:        updateVaultName,
				NameSet:     cmd.Flags().Changed("name"),
				Unlocked:    updateVaultUnlocked,
				UnlockedSet: cmd.Flags().Changed("unlocked"),
				Json:        useJsonOutput(),
			})
			return err
		},
		Short: "Update a vault",
		Use:   "update NAME_OR_ID [flags]",
	}
)

var (
	deleteVaultYes bool

	vaultsDeleteCmd = &cobra.Command{
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := service.DeleteVault(cli.DeleteVaultConfig{
				Vault: args[0],
				Json:  useJsonOutput(),
				Yes:   deleteVaultYes,
			})
			return err
		},
		Short: "Delete a vault",
		Use:   "delete NAME_OR_ID [flags]",
	}
)

// --- secrets subcommand group ---

var vaultsSecretsCmd = &cobra.Command{
	Short: "Manage secrets in a vault",
	Use:   "secrets",
}

var (
	secretsListVault string

	vaultsSecretsListCmd = &cobra.Command{
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := service.ListSecrets(cli.ListSecretsConfig{
				Vault: secretsListVault,
				Json:  useJsonOutput(),
			})
			return err
		},
		Short: "List secrets in a vault",
		Use:   "list",
	}
)

var (
	secretsSetVault string
	secretsSetFile  string

	vaultsSecretsSetCmd = &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			var secrets []string
			if len(args) >= 0 {
				secrets = args
			}

			useJson := useJsonOutput()
			_, err := service.SetSecretsInVault(cli.SetSecretsInVaultConfig{
				Vault:   secretsSetVault,
				File:    secretsSetFile,
				Secrets: secrets,
				Json:    useJson,
			})
			return err
		},
		Short: "Set secrets in a vault",
		Use:   "set [flags] [SECRETNAME=secretvalue]",
	}
)

var (
	secretsDeleteVault string
	secretsDeleteYes   bool

	vaultsSecretsDeleteCmd = &cobra.Command{
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			useJson := useJsonOutput()
			_, err := service.DeleteSecret(cli.DeleteSecretConfig{
				SecretName: args[0],
				Vault:      secretsDeleteVault,
				Json:       useJson,
				Yes:        secretsDeleteYes,
			})
			return err
		},
		Short: "Delete a secret from a vault",
		Use:   "delete NAME [flags]",
	}
)

// --- vars subcommand group ---

var vaultsVarsCmd = &cobra.Command{
	Short: "Manage vars in a vault",
	Use:   "vars",
}

var (
	varsListVault string

	vaultsVarsListCmd = &cobra.Command{
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := service.ListVars(cli.ListVarsConfig{
				Vault: varsListVault,
				Json:  useJsonOutput(),
			})
			return err
		},
		Short: "List vars in a vault",
		Use:   "list",
	}
)

var (
	varsSetVault string
	varsSetFile  string

	vaultsVarsSetCmd = &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			var vars []string
			if len(args) >= 0 {
				vars = args
			}

			useJson := useJsonOutput()
			_, err := service.SetVars(cli.SetVarsConfig{
				Vault: varsSetVault,
				File:  varsSetFile,
				Vars:  vars,
				Json:  useJson,
			})
			return err
		},
		Short: "Set vars in a vault",
		Use:   "set [flags] [KEY=value]",
	}
)

var (
	varsShowVault string

	vaultsVarsShowCmd = &cobra.Command{
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			useJson := useJsonOutput()
			_, err := service.ShowVar(cli.ShowVarConfig{
				VarName: args[0],
				Vault:   varsShowVault,
				Json:    useJson,
			})
			return err
		},
		Short: "Show a var from a vault",
		Use:   "show NAME [flags]",
	}
)

var (
	varsDeleteVault string
	varsDeleteYes   bool

	vaultsVarsDeleteCmd = &cobra.Command{
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			useJson := useJsonOutput()
			_, err := service.DeleteVar(cli.DeleteVarConfig{
				VarName: args[0],
				Vault:   varsDeleteVault,
				Json:    useJson,
				Yes:     varsDeleteYes,
			})
			return err
		},
		Short: "Delete a var from a vault",
		Use:   "delete NAME [flags]",
	}
)

// --- oidc-tokens subcommand group ---

var vaultsOidcTokensCmd = &cobra.Command{
	Short: "Manage OIDC tokens in a vault",
	Use:   "oidc-tokens",
}

var (
	oidcTokenListVault string

	vaultsOidcTokensListCmd = &cobra.Command{
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := service.ListVaultOidcTokens(cli.ListVaultOidcTokensConfig{
				Vault: oidcTokenListVault,
				Json:  useJsonOutput(),
			})
			return err
		},
		Short: "List OIDC tokens in a vault",
		Use:   "list [flags]",
	}
)

var (
	oidcTokenCreateVault    string
	oidcTokenCreateName     string
	oidcTokenCreateAudience string
	oidcTokenCreateProvider string

	vaultsOidcTokensCreateCmd = &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			useJson := useJsonOutput()
			_, err := service.CreateVaultOidcToken(cli.CreateVaultOidcTokenConfig{
				Vault:    oidcTokenCreateVault,
				Name:     oidcTokenCreateName,
				Audience: oidcTokenCreateAudience,
				Provider: oidcTokenCreateProvider,
				Json:     useJson,
			})
			return err
		},
		Short: "Create an OIDC token in a vault",
		Use:   "create [flags]",
	}
)

var (
	oidcTokenShowVault string

	vaultsOidcTokensShowCmd = &cobra.Command{
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := service.ShowVaultOidcToken(cli.VaultOidcTokenConfig{
				Vault: oidcTokenShowVault,
				Token: args[0],
				Json:  useJsonOutput(),
			})
			return err
		},
		Short: "Show an OIDC token",
		Use:   "show NAME_OR_ID [flags]",
	}
)

var (
	oidcTokenUpdateVault    string
	oidcTokenUpdateName     string
	oidcTokenUpdateAudience string

	vaultsOidcTokensUpdateCmd = &cobra.Command{
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := service.UpdateVaultOidcToken(cli.UpdateVaultOidcTokenConfig{
				Vault:    oidcTokenUpdateVault,
				Token:    args[0],
				Name:     oidcTokenUpdateName,
				Audience: oidcTokenUpdateAudience,
				Json:     useJsonOutput(),
			})
			return err
		},
		Short: "Update an OIDC token",
		Use:   "update NAME_OR_ID [flags]",
	}
)

var (
	oidcTokenDeleteVault string
	oidcTokenDeleteYes   bool

	vaultsOidcTokensDeleteCmd = &cobra.Command{
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := service.DeleteVaultOidcToken(cli.DeleteVaultOidcTokenConfig{
				Vault: oidcTokenDeleteVault,
				Token: args[0],
				Json:  useJsonOutput(),
				Yes:   oidcTokenDeleteYes,
			})
			return err
		},
		Short: "Delete an OIDC token",
		Use:   "delete NAME_OR_ID [flags]",
	}
)

var vaultsApproversCmd = &cobra.Command{
	Short: "Manage approvers for a vault",
	Use:   "approvers",
}

var (
	approversListVault string

	vaultsApproversListCmd = &cobra.Command{
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := service.ListVaultApprovers(cli.ListVaultApproversConfig{
				Vault: approversListVault,
				Json:  useJsonOutput(),
			})
			return err
		},
		Short: "List approvers for a vault",
		Use:   "list [flags]",
	}
)

var (
	approversAddVault string

	vaultsApproversAddCmd = &cobra.Command{
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := service.AddVaultApprover(cli.AddVaultApproverConfig{
				Vault: approversAddVault,
				Email: args[0],
				Json:  useJsonOutput(),
			})
			return err
		},
		Short: "Add an approver to a vault",
		Use:   "add EMAIL [flags]",
	}
)

var (
	approversRemoveVault string
	approversRemoveYes   bool

	vaultsApproversRemoveCmd = &cobra.Command{
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := service.RemoveVaultApprover(cli.RemoveVaultApproverConfig{
				Vault:      approversRemoveVault,
				ApproverID: args[0],
				Json:       useJsonOutput(),
				Yes:        approversRemoveYes,
			})
			return err
		},
		Short: "Remove an approver from a vault",
		Use:   "remove APPROVER_ID [flags]",
	}
)

// --- set-secrets alias (backwards compatibility) ---

var (
	setSecretsVault string
	setSecretsFile  string

	vaultsSetSecretsCmd = &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			var secrets []string
			if len(args) >= 0 {
				secrets = args
			}

			useJson := useJsonOutput()
			_, err := service.SetSecretsInVault(cli.SetSecretsInVaultConfig{
				Vault:   setSecretsVault,
				File:    setSecretsFile,
				Secrets: secrets,
				Json:    useJson,
			})
			return err
		},
		Hidden: true,
		Short:  "Set secrets in a vault",
		Use:    "set-secrets [flags] [SECRETNAME=secretvalue]",
	}
)

func init() {
	vaultsCmd.AddCommand(vaultsListCmd)

	// vaults create
	vaultsCreateCmd.Flags().StringVar(&createVaultName, "name", "", "the name of the vault to create")
	_ = vaultsCreateCmd.MarkFlagRequired("name")
	vaultsCreateCmd.Flags().BoolVar(&createVaultUnlocked, "unlocked", false, "whether the vault should be unlocked")
	vaultsCreateCmd.Flags().StringSliceVar(&createVaultRepoPerms, "repository-permission", nil, "repository permission in the format REPO_SLUG:REF_PATTERN (repeatable)")
	vaultsCmd.AddCommand(vaultsCreateCmd)

	vaultsCmd.AddCommand(vaultsShowCmd)

	vaultsUpdateCmd.Flags().StringVar(&updateVaultName, "name", "", "the new name of the vault")
	vaultsUpdateCmd.Flags().BoolVar(&updateVaultUnlocked, "unlocked", false, "whether the vault should be unlocked")
	vaultsCmd.AddCommand(vaultsUpdateCmd)

	vaultsDeleteCmd.Flags().BoolVarP(&deleteVaultYes, "yes", "y", false, "skip confirmation prompt")
	vaultsCmd.AddCommand(vaultsDeleteCmd)

	// vaults secrets set
	vaultsSecretsSetCmd.Flags().StringVar(&secretsSetVault, "vault", "default", "the name of the vault to set the secrets in")
	vaultsSecretsSetCmd.Flags().StringVar(&secretsSetFile, "file", "", "the path to a file in dotenv format to read the secrets from")
	vaultsSecretsCmd.AddCommand(vaultsSecretsSetCmd)

	vaultsSecretsListCmd.Flags().StringVar(&secretsListVault, "vault", "default", "the name of the vault to list secrets from")
	vaultsSecretsCmd.AddCommand(vaultsSecretsListCmd)

	// vaults secrets delete
	vaultsSecretsDeleteCmd.Flags().StringVar(&secretsDeleteVault, "vault", "default", "the name of the vault to delete the secret from")
	vaultsSecretsDeleteCmd.Flags().BoolVarP(&secretsDeleteYes, "yes", "y", false, "skip confirmation prompt")
	vaultsSecretsCmd.AddCommand(vaultsSecretsDeleteCmd)

	vaultsCmd.AddCommand(vaultsSecretsCmd)

	// vaults vars set
	vaultsVarsSetCmd.Flags().StringVar(&varsSetVault, "vault", "default", "the name of the vault to set the vars in")
	vaultsVarsSetCmd.Flags().StringVar(&varsSetFile, "file", "", "the path to a file in dotenv format to read the vars from")
	vaultsVarsCmd.AddCommand(vaultsVarsSetCmd)

	vaultsVarsListCmd.Flags().StringVar(&varsListVault, "vault", "default", "the name of the vault to list vars from")
	vaultsVarsCmd.AddCommand(vaultsVarsListCmd)

	// vaults vars show
	vaultsVarsShowCmd.Flags().StringVar(&varsShowVault, "vault", "default", "the name of the vault to show the var from")
	vaultsVarsCmd.AddCommand(vaultsVarsShowCmd)

	// vaults vars delete
	vaultsVarsDeleteCmd.Flags().StringVar(&varsDeleteVault, "vault", "default", "the name of the vault to delete the var from")
	vaultsVarsDeleteCmd.Flags().BoolVarP(&varsDeleteYes, "yes", "y", false, "skip confirmation prompt")
	vaultsVarsCmd.AddCommand(vaultsVarsDeleteCmd)

	vaultsCmd.AddCommand(vaultsVarsCmd)

	vaultsOidcTokensListCmd.Flags().StringVar(&oidcTokenListVault, "vault", "", "the name or ID of the vault to list OIDC tokens from")
	_ = vaultsOidcTokensListCmd.MarkFlagRequired("vault")
	vaultsOidcTokensCmd.AddCommand(vaultsOidcTokensListCmd)

	vaultsOidcTokensCreateCmd.Flags().StringVar(&oidcTokenCreateVault, "vault", "", "the name of the vault to create the OIDC token in")
	_ = vaultsOidcTokensCreateCmd.MarkFlagRequired("vault")
	vaultsOidcTokensCreateCmd.Flags().StringVar(&oidcTokenCreateName, "name", "", "the name of the OIDC token (required unless --provider is given)")
	vaultsOidcTokensCreateCmd.Flags().StringVar(&oidcTokenCreateAudience, "audience", "", "the audience for the OIDC token (required unless --provider is given; always required for gcp)")
	vaultsOidcTokensCreateCmd.Flags().StringVar(&oidcTokenCreateProvider, "provider", "", "use defaults for a known provider (e.g. aws, gcp); sets name and audience automatically")
	vaultsOidcTokensCmd.AddCommand(vaultsOidcTokensCreateCmd)

	vaultsOidcTokensShowCmd.Flags().StringVar(&oidcTokenShowVault, "vault", "", "the name or ID of the vault containing the OIDC token")
	_ = vaultsOidcTokensShowCmd.MarkFlagRequired("vault")
	vaultsOidcTokensCmd.AddCommand(vaultsOidcTokensShowCmd)

	vaultsOidcTokensUpdateCmd.Flags().StringVar(&oidcTokenUpdateVault, "vault", "", "the name or ID of the vault containing the OIDC token")
	_ = vaultsOidcTokensUpdateCmd.MarkFlagRequired("vault")
	vaultsOidcTokensUpdateCmd.Flags().StringVar(&oidcTokenUpdateName, "name", "", "the new name for the OIDC token")
	vaultsOidcTokensUpdateCmd.Flags().StringVar(&oidcTokenUpdateAudience, "audience", "", "the new audience for the OIDC token")
	vaultsOidcTokensCmd.AddCommand(vaultsOidcTokensUpdateCmd)

	vaultsOidcTokensDeleteCmd.Flags().StringVar(&oidcTokenDeleteVault, "vault", "", "the name or ID of the vault containing the OIDC token")
	_ = vaultsOidcTokensDeleteCmd.MarkFlagRequired("vault")
	vaultsOidcTokensDeleteCmd.Flags().BoolVarP(&oidcTokenDeleteYes, "yes", "y", false, "skip confirmation prompt")
	vaultsOidcTokensCmd.AddCommand(vaultsOidcTokensDeleteCmd)

	vaultsCmd.AddCommand(vaultsOidcTokensCmd)

	vaultsApproversListCmd.Flags().StringVar(&approversListVault, "vault", "", "the name or ID of the vault")
	_ = vaultsApproversListCmd.MarkFlagRequired("vault")
	vaultsApproversCmd.AddCommand(vaultsApproversListCmd)
	vaultsApproversAddCmd.Flags().StringVar(&approversAddVault, "vault", "", "the name or ID of the vault")
	_ = vaultsApproversAddCmd.MarkFlagRequired("vault")
	vaultsApproversCmd.AddCommand(vaultsApproversAddCmd)
	vaultsApproversRemoveCmd.Flags().StringVar(&approversRemoveVault, "vault", "", "the name or ID of the vault")
	_ = vaultsApproversRemoveCmd.MarkFlagRequired("vault")
	vaultsApproversRemoveCmd.Flags().BoolVarP(&approversRemoveYes, "yes", "y", false, "skip confirmation prompt")
	vaultsApproversCmd.AddCommand(vaultsApproversRemoveCmd)
	vaultsCmd.AddCommand(vaultsApproversCmd)

	// vaults set-secrets (alias for backwards compatibility)
	vaultsSetSecretsCmd.Flags().StringVar(&setSecretsVault, "vault", "default", "the name of the vault to set the secrets in")
	vaultsSetSecretsCmd.Flags().StringVar(&setSecretsFile, "file", "", "the path to a file in dotenv format to read the secrets from")
	vaultsCmd.AddCommand(vaultsSetSecretsCmd)
}
