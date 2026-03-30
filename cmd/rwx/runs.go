package main

import "github.com/spf13/cobra"

var runsCmd *cobra.Command

func init() {
	runsCmd = &cobra.Command{
		GroupID: "execution",
		Use:     "runs",
		Short:   "Manage runs",
	}

	runsCreateCmd := &cobra.Command{
		Use:     "create <file> [flags]",
		Short:   "Launch a run from a local RWX definitions file",
		Args:    runCmd.Args,
		PreRunE: runCmd.PreRunE,
		RunE:    runCmd.RunE,
	}
	runsCreateCmd.Flags().AddFlagSet(runCmd.Flags())

	runsResultsCmd := &cobra.Command{
		Use:     "results [run-id]",
		Short:   "Get results for a run",
		Args:    resultsCmd.Args,
		PreRunE: resultsCmd.PreRunE,
		RunE:    resultsCmd.RunE,
	}
	runsResultsCmd.Flags().AddFlagSet(resultsCmd.Flags())

	runsGetCmd := &cobra.Command{
		Use:     "get [run-id]",
		Short:   "Get results for a run",
		Args:    resultsCmd.Args,
		PreRunE: resultsCmd.PreRunE,
		RunE:    resultsCmd.RunE,
		Hidden:  true,
	}
	runsGetCmd.Flags().AddFlagSet(resultsCmd.Flags())

	runsShowCmd := &cobra.Command{
		Use:     "show [run-id]",
		Short:   "Get results for a run",
		Args:    resultsCmd.Args,
		PreRunE: resultsCmd.PreRunE,
		RunE:    resultsCmd.RunE,
		Hidden:  true,
	}
	runsShowCmd.Flags().AddFlagSet(resultsCmd.Flags())

	runsCmd.AddCommand(runsCreateCmd)
	runsCmd.AddCommand(runsResultsCmd)
	runsCmd.AddCommand(runsGetCmd)
	runsCmd.AddCommand(runsShowCmd)

	rootCmd.AddCommand(runsCmd)
}
