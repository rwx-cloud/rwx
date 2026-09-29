package main

import (
	"os"

	"github.com/rwx-cloud/rwx/internal/accesstoken"
	"github.com/rwx-cloud/rwx/internal/lsp"
	"github.com/spf13/cobra"
)

var (
	lspCmd = &cobra.Command{
		GroupID: "setup",
		Use:     "lsp",
		Short:   "LSP (Language Server Protocol) related commands",
	}

	lspServeCmd = &cobra.Command{
		Use:   "serve",
		Short: "Start an LSP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := accesstoken.Get(accessTokenBackend, AccessToken)
			if err != nil {
				return err
			}
			exitCode, err := lsp.Serve(telemetryCollector, token)
			if err != nil {
				return err
			}

			os.Exit(exitCode)
			return nil
		},
	}
)

func init() {
	lspCmd.AddCommand(lspServeCmd)
}
