package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/rwx-cloud/rwx/internal/cli"

	"github.com/spf13/cobra"
)

var runsCancelCmd = &cobra.Command{
	Use:   "cancel <run-id>",
	Short: "Cancel a run",
	Long:  `Cancel a run that is in progress.`,
	Args:  cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return requireAccessToken()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := service.CancelRun(cli.CancelRunConfig{RunID: args[0]})
		if err != nil {
			return err
		}

		if useJsonOutput() {
			encoded, err := json.Marshal(result)
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stdout, string(encoded))
			return nil
		}

		fmt.Fprintf(os.Stdout, "Cancelled run %s\n", result.RunID)
		return nil
	},
}

var cancelCmd = &cobra.Command{
	GroupID: "execution",
	Use:     "cancel <run-id>",
	Short:   runsCancelCmd.Short,
	Long:    runsCancelCmd.Long,
	Args:    runsCancelCmd.Args,
	PreRunE: runsCancelCmd.PreRunE,
	RunE:    runsCancelCmd.RunE,
}
