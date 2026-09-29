package main

import (
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/spf13/cobra"
)

var packagesCmd = &cobra.Command{
	Aliases: []string{"package"},
	GroupID: "definitions",
	Hidden:  true,
	Short:   "Manage RWX packages",
	Use:     "packages",
}

var (
	PackagesAllowMajorVersionChange bool
	PackagesBuildTimestamp          string
	PackagesBuildPublish            bool
	PackagesShowNoReadme            bool

	packagesBuildCmd = &cobra.Command{
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			directory := "."
			if len(args) == 1 {
				directory = args[0]
			}

			_, err := service.BuildPackage(cli.PackageBuildConfig{
				Directory: directory,
				Timestamp: PackagesBuildTimestamp,
				Publish:   PackagesBuildPublish,
				Json:      useJsonOutput(),
			})
			return err
		},
		Short: "Build and upload a package",
		Long: "Build and upload a package.\n" +
			"Zips the contents of the given directory (the current directory by default), " +
			"uploads it to RWX, and prints the resulting content digest.\n" +
			"Use --publish to publish the uploaded digest. " +
			"Visibility is read from the package manifest (public when omitted); publishing does not change it.",
		Use: "build [flags] [directory]",
	}

	packagesPublishCmd = &cobra.Command{
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := service.PublishPackage(cli.PackagePublishConfig{Digest: args[0], Json: useJsonOutput()})
			return err
		},
		Short: "Publish an uploaded package",
		Long:  "Publish an uploaded package by digest without changing its manifest visibility.",
		Use:   "publish [flags] <digest>",
	}

	packagesListCmd = &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := service.ListPackages(cli.ListPackagesConfig{
				Json: useJsonOutput(),
			})
			return err
		},
		Short: "List all available packages and their latest versions",
		Use:   "list",
		Args:  cobra.NoArgs,
	}

	packagesShowCmd = &cobra.Command{
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := service.ShowPackage(cli.ShowPackageConfig{
				PackageName: args[0],
				Json:        useJsonOutput(),
				NoReadme:    PackagesShowNoReadme,
			})
			return err
		},
		Short: "Show details for a package",
		Use:   "show [flags] <package-name>",
	}

	packagesUpdateCmd = &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			replacementVersionPicker := cli.PickLatestMinorVersion
			if PackagesAllowMajorVersionChange {
				replacementVersionPicker = cli.PickLatestMajorVersion
			}

			useJson := useJsonOutput()
			_, err := service.UpdatePackages(cli.UpdatePackagesConfig{
				Files:                    args,
				RwxDirectory:             RwxDirectory,
				ReplacementVersionPicker: replacementVersionPicker,
				Json:                     useJson,
			})
			return err
		},
		Short: "Update all packages to their latest (minor) version",
		Long: "Update all packages to their latest (minor) version.\n" +
			"Takes a list of files as arguments, or updates all toplevel YAML files in .rwx if no files are given.",
		Use: "update [flags] [file...]",
	}
)

func init() {
	packagesBuildCmd.Flags().StringVar(&PackagesBuildTimestamp, "timestamp", "", "normalize file modification times in the zip to this `timestamp` (format: YYYYMMDDHHmm) for reproducible builds")
	packagesBuildCmd.Flags().BoolVar(&PackagesBuildPublish, "publish", false, "publish the package after uploading it")
	packagesShowCmd.Flags().BoolVar(&PackagesShowNoReadme, "no-readme", false, "hide the readme documentation")
	packagesUpdateCmd.Flags().BoolVar(&PackagesAllowMajorVersionChange, "allow-major-version-change", false, "update packages to the latest major version")
	addRwxDirFlag(packagesUpdateCmd)
	packagesCmd.AddCommand(packagesBuildCmd)
	packagesCmd.AddCommand(packagesPublishCmd)
	packagesCmd.AddCommand(packagesListCmd)
	packagesCmd.AddCommand(packagesShowCmd)
	packagesCmd.AddCommand(packagesUpdateCmd)
}
