package main

import (
	"github.com/rwx-cloud/rwx/internal/cli"

	"github.com/spf13/cobra"
)

var whoamiCmd = &cobra.Command{
	GroupID: "setup",
	RunE: func(cmd *cobra.Command, args []string) error {
		useJson := useJsonOutput()
		_, err := service.Whoami(cli.WhoamiConfig{Json: useJson})
		return err
	},
	Short: "Outputs details about the access token in use",
	Use:   "whoami [flags]",
}

func init() {
}
