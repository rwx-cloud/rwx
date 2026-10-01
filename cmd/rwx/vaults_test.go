package main

import (
	"testing"

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
