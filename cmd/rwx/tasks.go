package main

import "github.com/spf13/cobra"

var tasksCmd = &cobra.Command{
	GroupID: "execution",
	Use:     "tasks",
	Short:   "Manage tasks",
}

func init() {
	tasksCmd.AddCommand(tasksRetryCmd)
	rootCmd.AddCommand(tasksCmd)
}
