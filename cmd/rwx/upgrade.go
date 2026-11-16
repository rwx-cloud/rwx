package main

import (
	"github.com/rwx-cloud/rwx/internal/cli"

	"github.com/spf13/cobra"
)

var (
	UpgradeVersion string
	UpgradeCheck   bool
	UpgradeForce   bool

	upgradeCmd = &cobra.Command{
		GroupID: "setup",
		RunE: func(cmd *cobra.Command, args []string) error {
			useJson := useJsonOutput()
			_, err := service.Upgrade(cli.UpgradeConfig{
				Version: UpgradeVersion,
				Check:   UpgradeCheck,
				Force:   UpgradeForce,
				Json:    useJson,
			})
			return err
		},
		Short: "Upgrade the RWX CLI to the latest release",
		Long: "Upgrade the RWX CLI to the latest release.\n" +
			"Downloads the release binary for this platform from GitHub, verifies its checksum, and replaces the running executable.\n" +
			"Installations managed by Homebrew should be upgraded with `brew upgrade rwx-cloud/tap/rwx` instead.",
		Use: "upgrade [flags]",
	}
)

func init() {
	upgradeCmd.Flags().StringVar(&UpgradeVersion, "version", "", "upgrade (or downgrade) to a specific version instead of the latest release")
	upgradeCmd.Flags().BoolVar(&UpgradeCheck, "check", false, "only check for a new release; exits 1 if one is available")
	upgradeCmd.Flags().BoolVar(&UpgradeForce, "force", false, "reinstall even if already up to date")
}
