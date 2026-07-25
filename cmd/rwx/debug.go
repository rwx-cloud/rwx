package main

import (
	"github.com/rwx-cloud/rwx/internal/cli"

	"github.com/spf13/cobra"
)

var DebugSession string

var debugCmd = &cobra.Command{
	GroupID: "execution",
	Args:    cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return requireAccessToken()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return service.DebugTask(cli.DebugTaskConfig{DebugKey: args[0], Session: DebugSession})
	},
	Short: "Debug a task",
	Use:   "debug [flags] [debugKey]",
}

func init() {
	debugCmd.Flags().StringVar(&DebugSession, "session", "", "select a breakpoint session by ID or unique open name")
}
