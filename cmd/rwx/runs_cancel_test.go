package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunsCancelIsRegistered(t *testing.T) {
	cancelCmd := findSubcommand(runsCmd, "cancel")
	require.NotNil(t, cancelCmd, "rwx runs cancel should exist")
	require.False(t, cancelCmd.Hidden, "cancel is a real action and should be visible")
}

func TestRunsCancelRequiresExactlyOneRunID(t *testing.T) {
	cancelCmd := findSubcommand(runsCmd, "cancel")
	require.NotNil(t, cancelCmd)

	require.Error(t, cancelCmd.Args(cancelCmd, []string{}), "cancel should reject a missing run ID")
	require.NoError(t, cancelCmd.Args(cancelCmd, []string{"run-1"}))
	require.Error(t, cancelCmd.Args(cancelCmd, []string{"run-1", "run-2"}), "cancel should reject a second run ID")
}

func TestCancelMirrorsRunsCancel(t *testing.T) {
	topLevel := findSubcommand(rootCmd, "cancel")
	require.NotNil(t, topLevel, "rwx cancel should exist")
	require.False(t, topLevel.Hidden, "cancel is a real action and should be visible")
	require.Equal(t, "execution", topLevel.GroupID)

	require.True(t, sameFunc(topLevel.RunE, runsCancelCmd.RunE), "cancel should reuse runs cancel's RunE")
	require.True(t, sameFunc(topLevel.PreRunE, runsCancelCmd.PreRunE), "cancel should reuse runs cancel's PreRunE")
	require.Equal(t, runsCancelCmd.Short, topLevel.Short)
	require.Equal(t, runsCancelCmd.Long, topLevel.Long)

	require.Error(t, topLevel.Args(topLevel, []string{}), "cancel should reject a missing run ID")
	require.NoError(t, topLevel.Args(topLevel, []string{"run-1"}))
	require.Error(t, topLevel.Args(topLevel, []string{"run-1", "run-2"}), "cancel should reject a second run ID")
}
