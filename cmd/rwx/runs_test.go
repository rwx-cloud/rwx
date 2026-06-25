package main

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func findSubcommand(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

func sameFunc(a, b any) bool {
	return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}

func TestRunsIsPureParent(t *testing.T) {
	// A pure parent: no Run/RunE means Cobra prints help by default and a bare
	// `rwx runs` can never initiate a run or alias `rwx results`.
	require.Nil(t, runsCmd.RunE)
	require.Nil(t, runsCmd.Run)
	require.False(t, runsCmd.Runnable())
	require.True(t, runsCmd.HasSubCommands())
}

func TestRunsStartMirrorsRun(t *testing.T) {
	startCmd := findSubcommand(runsCmd, "start")
	require.NotNil(t, startCmd, "rwx runs start should exist")
	require.False(t, startCmd.Hidden, "start is a real action and should be visible")

	// start is the noun-verb form of `rwx run`: it reuses runCmd's validation
	// and execution so the two stay in lockstep.
	require.True(t, sameFunc(startCmd.RunE, runCmd.RunE), "start should reuse run's RunE")
	require.True(t, sameFunc(startCmd.PreRunE, runCmd.PreRunE), "start should reuse run's PreRunE")

	// It exposes the same flags as `rwx run`.
	for _, name := range []string{"init", "target", "file", "no-cache", "dir", "open", "debug", "wait", "fail-fast", "title"} {
		require.NotNil(t, startCmd.Flags().Lookup(name), "start should expose the --%s flag", name)
	}
}

func TestRunsShowIsVisibleAndAliasesResults(t *testing.T) {
	// show is the visible single-run inspection command and reuses results'
	// execution so the two stay in lockstep.
	showCmd := findSubcommand(runsCmd, "show")
	require.NotNil(t, showCmd, "rwx runs show should exist")
	require.False(t, showCmd.Hidden, "rwx runs show should be visible for discoverability")
	require.True(t, sameFunc(showCmd.RunE, resultsCmd.RunE), "rwx runs show should alias results")
	require.Equal(t, resultsCmd.Short, showCmd.Short)
}

func TestRunsGetIsAnAliasOfShow(t *testing.T) {
	// `get` resolves to show via a Cobra alias rather than its own command, so
	// `rwx runs get` works without a second line under `rwx runs -h`.
	require.Nil(t, findSubcommand(runsCmd, "get"), "rwx runs get should not be its own command")
	showCmd := findSubcommand(runsCmd, "show")
	require.NotNil(t, showCmd)
	require.Contains(t, showCmd.Aliases, "get", "show should expose get as an alias")
}

// executeRoot runs the root command with the given args, capturing combined
// output. A bare/parent invocation resolves to ErrHelp before PersistentPreRunE,
// so no network or access token is required.
func executeRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs(args)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
	})
	err := rootCmd.Execute()
	return buf.String(), err
}

func TestRunsBarePrintsHelp(t *testing.T) {
	out, err := executeRoot(t, "runs")
	require.NoError(t, err)
	require.Contains(t, out, "Available Commands")
	require.Contains(t, out, "start")
}

func TestRunsBareIdPrintsHelpAndDoesNotAliasResults(t *testing.T) {
	// The bare-id form resolves to the parent (non-runnable), so Cobra prints
	// help instead of looking up a run; it is not a `rwx results` alias.
	out, err := executeRoot(t, "runs", "some-run-id")
	require.NoError(t, err)
	require.Contains(t, out, "Available Commands")
	require.NotContains(t, out, "result status")
}
