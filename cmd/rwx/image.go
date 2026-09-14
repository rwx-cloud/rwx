package main

import (
	"github.com/rwx-cloud/rwx/cmd/rwx/image"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/spf13/cobra"
)

var imageCmd = &cobra.Command{
	GroupID: "execution",
	Use:     "image",
	Short:   "Manage OCI images",
}

// for backcompat
var pushCmd *cobra.Command

func init() {
	image.InitPush(func() cli.Service { return service }, useJsonOutput)
	image.InitBuild(ParseInitParameters, func() cli.Service { return service }, useJsonOutput)
	image.InitPull(func() cli.Service { return service }, useJsonOutput)
	imageCmd.AddCommand(image.PushCmd)
	imageCmd.AddCommand(image.BuildCmd)
	imageCmd.AddCommand(image.PullCmd)

	// for backcompat
	pushCmd = &cobra.Command{
		GroupID: "execution",
		Args:    image.PushCmd.Args,
		PreRunE: image.PushCmd.PreRunE,
		RunE:    image.PushCmd.RunE,
		Short:   image.PushCmd.Short,
		Use:     image.PushCmd.Use,
		Hidden:  true,
	}
	pushCmd.Flags().AddFlagSet(image.PushCmd.Flags())
}
