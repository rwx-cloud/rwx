package main

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestVaultLifecycleCommands(t *testing.T) {
	for _, name := range []string{"show", "update", "delete"} {
		command := findSubcommand(vaultsCmd, name)
		require.NotNil(t, command)
		require.Error(t, command.Args(command, nil))
		require.NoError(t, command.Args(command, []string{"deploys"}))
		require.Error(t, command.Args(command, []string{"deploys", "production"}))
	}

	require.NotNil(t, vaultsUpdateCmd.Flags().Lookup("name"))
	require.NotNil(t, vaultsUpdateCmd.Flags().Lookup("unlocked"))
	require.NotNil(t, vaultsUpdateCmd.Flags().Lookup("approvals-enabled"))
	require.NotNil(t, vaultsUpdateCmd.Flags().Lookup("required-approvals"))
	require.NotNil(t, vaultsCreateCmd.Flags().Lookup("approvals-enabled"))
	require.NotNil(t, vaultsCreateCmd.Flags().Lookup("required-approvals"))
	require.Equal(t, "y", vaultsDeleteCmd.Flags().ShorthandLookup("y").Shorthand)
}

func TestVaultOidcTokenCommands(t *testing.T) {
	require.Same(t, vaultsOidcTokensCmd, findSubcommand(vaultsCmd, "oidc-tokens"))
	require.Len(t, vaultsOidcTokensCmd.Commands(), 5)

	list := findSubcommand(vaultsOidcTokensCmd, "list")
	require.NotNil(t, list)
	require.NoError(t, list.Args(list, nil))
	require.Error(t, list.Args(list, []string{"aws"}))
	require.ErrorContains(t, list.ValidateRequiredFlags(), "vault")

	create := findSubcommand(vaultsOidcTokensCmd, "create")
	require.NotNil(t, create)
	require.ErrorContains(t, create.ValidateRequiredFlags(), "vault")

	for _, name := range []string{"show", "update", "delete"} {
		command := findSubcommand(vaultsOidcTokensCmd, name)
		require.NotNil(t, command)
		require.Error(t, command.Args(command, nil))
		require.NoError(t, command.Args(command, []string{"aws"}))
		require.Error(t, command.Args(command, []string{"aws", "gcp"}))
		require.ErrorContains(t, command.ValidateRequiredFlags(), "vault")
	}

	require.NotNil(t, vaultsOidcTokensUpdateCmd.Flags().Lookup("name"))
	require.NotNil(t, vaultsOidcTokensUpdateCmd.Flags().Lookup("audience"))
	require.Equal(t, "y", vaultsOidcTokensDeleteCmd.Flags().ShorthandLookup("y").Shorthand)
}

func TestVaultApproverCommands(t *testing.T) {
	require.Same(t, vaultsApproversCmd, findSubcommand(vaultsCmd, "approvers"))
	require.Len(t, vaultsApproversCmd.Commands(), 3)

	list := findSubcommand(vaultsApproversCmd, "list")
	require.NoError(t, list.Args(list, nil))
	require.ErrorContains(t, list.ValidateRequiredFlags(), "vault")

	for _, name := range []string{"add", "remove"} {
		command := findSubcommand(vaultsApproversCmd, name)
		require.Error(t, command.Args(command, nil))
		require.NoError(t, command.Args(command, []string{"value"}))
		require.Error(t, command.Args(command, []string{"one", "two"}))
		require.ErrorContains(t, command.ValidateRequiredFlags(), "vault")
	}
	require.Equal(t, "y", vaultsApproversRemoveCmd.Flags().ShorthandLookup("y").Shorthand)
}

func TestVaultServiceAccountCommands(t *testing.T) {
	require.Same(t, vaultsServiceAccountsCmd, findSubcommand(vaultsCmd, "service-accounts"))
	require.Len(t, vaultsServiceAccountsCmd.Commands(), 3)

	list := findSubcommand(vaultsServiceAccountsCmd, "list")
	require.NoError(t, list.Args(list, nil))
	require.ErrorContains(t, list.ValidateRequiredFlags(), "vault")

	for _, name := range []string{"attach", "detach"} {
		command := findSubcommand(vaultsServiceAccountsCmd, name)
		require.Error(t, command.Args(command, nil))
		require.NoError(t, command.Args(command, []string{"value"}))
		require.Error(t, command.Args(command, []string{"one", "two"}))
		require.ErrorContains(t, command.ValidateRequiredFlags(), "vault")
	}
	require.Equal(t, "y", vaultsServiceAccountsDetachCmd.Flags().ShorthandLookup("y").Shorthand)
}

func TestVaultAccessCommands(t *testing.T) {
	require.Same(t, vaultsAccessCmd, findSubcommand(vaultsCmd, "access"))
	require.Len(t, vaultsAccessCmd.Commands(), 3)

	list := findSubcommand(vaultsAccessCmd, "list")
	require.NotNil(t, list)
	require.NoError(t, list.Args(list, nil))
	require.Error(t, list.Args(list, []string{"rwx-cloud/cloud"}))
	require.ErrorContains(t, list.ValidateRequiredFlags(), "vault")

	for _, name := range []string{"allow", "revoke"} {
		command := findSubcommand(vaultsAccessCmd, name)
		require.NotNil(t, command)
		require.NoError(t, command.Args(command, nil))
		require.Error(t, command.Args(command, []string{"rwx-cloud/cloud"}))
		require.ErrorContains(t, command.ValidateRequiredFlags(), "vault")
		require.NotNil(t, command.Flags().Lookup("email"))
		require.NotNil(t, command.Flags().Lookup("service-account"))
	}

	require.NotNil(t, vaultsAccessAllowCmd.Flags().Lookup("allow-for"))
	require.NotNil(t, vaultsAccessAllowCmd.Flags().Lookup("expires-at"))
	require.Equal(t, "y", vaultsAccessRevokeCmd.Flags().ShorthandLookup("y").Shorthand)
}

func TestVaultAccessSubjectFlagGroups(t *testing.T) {
	for _, command := range []*cobra.Command{vaultsAccessAllowCmd, vaultsAccessRevokeCmd} {
		flagNames := []string{"repository-slug", "repository-branch-pattern", "email", "service-account"}
		original := make(map[string]bool, len(flagNames))
		for _, name := range flagNames {
			flag := command.Flags().Lookup(name)
			original[name] = flag.Changed
			flag.Changed = false
		}
		t.Cleanup(func() {
			for _, name := range flagNames {
				command.Flags().Lookup(name).Changed = original[name]
			}
		})

		require.Error(t, command.ValidateFlagGroups())
		command.Flags().Lookup("email").Changed = true
		require.NoError(t, command.ValidateFlagGroups())
		command.Flags().Lookup("service-account").Changed = true
		require.Error(t, command.ValidateFlagGroups())
		command.Flags().Lookup("email").Changed = false
		command.Flags().Lookup("service-account").Changed = false
		command.Flags().Lookup("repository-slug").Changed = true
		command.Flags().Lookup("repository-branch-pattern").Changed = true
		require.NoError(t, command.ValidateFlagGroups())
	}

	allowFor := vaultsAccessAllowCmd.Flags().Lookup("allow-for")
	expiresAt := vaultsAccessAllowCmd.Flags().Lookup("expires-at")
	originalAllowFor := allowFor.Changed
	originalExpiresAt := expiresAt.Changed
	t.Cleanup(func() {
		allowFor.Changed = originalAllowFor
		expiresAt.Changed = originalExpiresAt
	})
	allowFor.Changed = true
	expiresAt.Changed = true
	require.Error(t, vaultsAccessAllowCmd.ValidateFlagGroups())
}
