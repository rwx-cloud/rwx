package image

import (
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/skratchdot/open-golang/open"

	"github.com/spf13/cobra"
)

var (
	pushImageReferences  []string
	pushImageNoWait      bool
	pushImageOpen        bool
	pushImageCompression string

	PushCmd *cobra.Command
)

func InitPush(requireAccessToken func() error, getService func() cli.Service, useJsonOutput func() bool) {
	PushCmd = &cobra.Command{
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireAccessToken()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			openURL := open.Run
			if !pushImageOpen {
				openURL = func(input string) error { return nil }
			}

			config, err := cli.NewImagePushConfig(args[0], pushImageReferences, pushImageCompression, useJsonOutput(), !pushImageNoWait, openURL)
			if err != nil {
				return err
			}

			_, err = getService().ImagePush(config)
			return err
		},
		Short: "Push an RWX task to an OCI reference",
		Use:   "push <task-id> --to <reference> [--to <reference>] [--format json] [--open] [--no-wait]",
	}

	PushCmd.Flags().StringArrayVar(&pushImageReferences, "to", []string{}, "the qualified OCI reference to push the image to (can be specified multiple times)")
	PushCmd.Flags().StringVar(&pushImageCompression, "compression", "zstd", "compression format for newly pushed layers in the image (zstd, gzip, none)")
	PushCmd.Flags().BoolVar(&pushImageNoWait, "no-wait", false, "do not wait for the push to complete")
	PushCmd.Flags().BoolVar(&pushImageOpen, "open", false, "open the run URL in the default browser once the push starts")
}
