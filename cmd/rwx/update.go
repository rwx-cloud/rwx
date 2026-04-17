package main

import (
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	GroupID: "definitions",
	Short:   "Update versions for base layers and RWX packages",
	Use:     "update [flags] [files...]",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			switch args[0] {
			case "base":
				return updateBase(args[1:])
			case "packages":
				return updatePackages(args[1:])
			}
		}

		err := updateBase(args)
		if err != nil {
			return err
		}
		return updatePackages(args)
	},
}

var (
	AllowMajorVersionChange bool

	updateBaseCmd = &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateBase(args)
		},
		Short: "Update base images in RWX run configurations",
		Long: "Update base images in RWX run configurations.\n" +
			"Adds a base image to run configurations that don't have one, and migrates\n" +
			"deprecated 'os' and 'tag' fields to the new 'image' and 'config' format.",
		Use:    "base [flags] [files...]",
		Hidden: true,
	}

	updatePackagesCmd = &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			return updatePackages(args)
		},
		Short: "Update all packages to their latest (minor) version",
		Long: "Update all packages to their latest (minor) version.\n" +
			"Takes a list of files as arguments, or updates all toplevel YAML files in .rwx if no files are given.",
		Use: "packages [flags] [files...]",
	}
)

func updateBase(files []string) error {
	useJson := useJsonOutput()
	_, err := service.InsertBase(cli.InsertBaseConfig{
		Files:        files,
		RwxDirectory: RwxDirectory,
		Json:         useJson,
	})
	return err
}

func updatePackages(files []string) error {
	replacementVersionPicker := cli.PickLatestMinorVersion
	if AllowMajorVersionChange {
		replacementVersionPicker = cli.PickLatestMajorVersion
	}

	useJson := useJsonOutput()
	_, err := service.UpdatePackages(cli.UpdatePackagesConfig{
		Files:                    files,
		RwxDirectory:             RwxDirectory,
		ReplacementVersionPicker: replacementVersionPicker,
		Json:                     useJson,
	})
	return err
}

func init() {
	addRwxDirFlag(updateBaseCmd)

	updatePackagesCmd.Flags().BoolVar(&AllowMajorVersionChange, "allow-major-version-change", false, "update packages to the latest major version")
	addRwxDirFlag(updatePackagesCmd)

	updateCmd.Flags().BoolVar(&AllowMajorVersionChange, "allow-major-version-change", false, "update to the latest major version")
	updateCmd.AddCommand(updateBaseCmd)
	updateCmd.AddCommand(updatePackagesCmd)
	addRwxDirFlag(updateCmd)
}
