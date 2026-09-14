package main

import (
	"github.com/rwx-cloud/rwx/cmd/rwx/artifacts"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/spf13/cobra"
)

var artifactsCmd = &cobra.Command{
	GroupID: "outputs",
	Use:     "artifacts",
	Short:   "Manage task artifacts",
}

func init() {
	artifacts.InitDownload(func() cli.Service { return service }, useJsonOutput)
	artifacts.InitList(func() cli.Service { return service }, useJsonOutput)
	artifactsCmd.AddCommand(artifacts.DownloadCmd)
	artifactsCmd.AddCommand(artifacts.ListCmd)
}
