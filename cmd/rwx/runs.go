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

	// `rwx runs show` is the noun-verb form of `rwx results`: the single-run
	// inspection path, reusing results' execution so the two stay in lockstep.
	// `get` is an alias so coding-agent guesses (`rwx runs get`) resolve to the
	// same command without surfacing a second line under `rwx runs -h`.
	runsShowCmd := &cobra.Command{
		Use:     "show [run-id]",
		Aliases: []string{"get"},
		Short:   resultsCmd.Short,
		Long:    resultsCmd.Long,
		Args:    resultsCmd.Args,
		PreRunE: resultsCmd.PreRunE,
		RunE:    resultsCmd.RunE,
	}
	runsShowCmd.Flags().AddFlagSet(resultsCmd.Flags())

	runsCmd.AddCommand(runsStartCmd)
	runsCmd.AddCommand(runsShowCmd)
	runsCmd.AddCommand(runsListCmd)

	rootCmd.AddCommand(runsCmd)
}
