package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSSHCommand(t *testing.T) {
	cmd := findSubcommand(rootCmd, "ssh")
	require.NotNil(t, cmd, "rwx ssh should exist")
	require.Equal(t, "ssh [flags] [task-id]", cmd.Use)
	require.Equal(t, "execution", cmd.GroupID)
	require.NotNil(t, cmd.Flags().Lookup("name"), "rwx ssh should expose the --name flag")

	require.NoError(t, cmd.Args(cmd, []string{"task-123"}))
	require.Error(t, cmd.Args(cmd, nil))
	require.Error(t, cmd.Args(cmd, []string{"task-123", "extra"}))
}
