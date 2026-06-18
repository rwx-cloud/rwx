package main

import "github.com/spf13/cobra"

var runsCmd *cobra.Command

func init() {
	// `rwx runs` is a pure parent command. With no RunE and subcommands present,
	// Cobra prints help for a bare `rwx runs` (or any stray arg, e.g. a run id)
	// and never initiates a run, so the plural form can't be mistaken for
	// `rwx run` or for `rwx results`.
	runsCmd = &cobra.Command{
		GroupID: "execution",
		Use:     "runs",
		Short:   "Create and inspect runs",
	}

	// `rwx runs start` is the noun-verb form of `rwx run`. It reuses runCmd's
	// validation and execution so the two stay in lockstep.
	runsStartCmd := &cobra.Command{
		Use:     "start <file> [flags]",
		Short:   runCmd.Short,
		Long:    runCmd.Long,
		PreRunE: runCmd.PreRunE,
		RunE:    runCmd.RunE,
	}
	runsStartCmd.Flags().AddFlagSet(runCmd.Flags())

	aliasShort := "Alias for rwx results"

	runsGetCmd := &cobra.Command{
		Use:     "get [run-id]",
		Short:   aliasShort,
		Args:    resultsCmd.Args,
		PreRunE: resultsCmd.PreRunE,
		RunE:    resultsCmd.RunE,
		Hidden:  true,
	}
	runsGetCmd.Flags().AddFlagSet(resultsCmd.Flags())

	runsShowCmd := &cobra.Command{
		Use:     "show [run-id]",
		Short:   aliasShort,
		Args:    resultsCmd.Args,
		PreRunE: resultsCmd.PreRunE,
		RunE:    resultsCmd.RunE,
		Hidden:  true,
	}
	runsShowCmd.Flags().AddFlagSet(resultsCmd.Flags())

	runsCmd.AddCommand(runsStartCmd)
	runsCmd.AddCommand(runsGetCmd)
	runsCmd.AddCommand(runsShowCmd)

	rootCmd.AddCommand(runsCmd)
}
