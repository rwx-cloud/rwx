package main

import (
	"github.com/rwx-cloud/rwx/internal/cli"

	"github.com/spf13/cobra"
)

var SSHSessionName string

var sshCmd = &cobra.Command{
	GroupID: "execution",
	Args:    cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return requireAccessToken()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return service.AttachSSHSession(cli.AttachSSHSessionConfig{
			TaskID: args[0],
			Name:   SSHSessionName,
		})
	},
	Short: "Attach and connect to a running task",
	Use:   "ssh [flags] [task-id]",
}

func init() {
	sshCmd.Flags().StringVar(&SSHSessionName, "name", "", "name the attached SSH session")
}
