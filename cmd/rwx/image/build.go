package image

import (
	"fmt"
	"time"

	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
)

var (
	buildInitParameters   []string
	buildRwxDirectory     string
	buildMintFilePath     string
	buildNoCache          bool
	buildNoPull           bool
	buildTargetTaskKey    string
	buildTags             []string
	buildPushToReferences []string
	buildPushCompression  string
	buildTimeout          time.Duration
	buildOpen             bool

	BuildCmd *cobra.Command
)

func InitBuild(parseInitParameters func([]string) (map[string]string, error), getService func() cli.Service, useJsonOutput func() bool) {
	BuildCmd = &cobra.Command{
		Args: cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if buildNoPull && len(buildTags) > 0 {
				return fmt.Errorf("cannot use --tag with --no-pull: no image will be pulled to tag")
			}

			if cmd.Flags().Changed("push-compression") && len(buildPushToReferences) == 0 {
				return fmt.Errorf("cannot use --push-compression without --push-to: no image will be pushed")
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				buildMintFilePath = args[0]
			}

			initParams, err := parseInitParameters(buildInitParameters)
			if err != nil {
				return err
			}

			openURL := open.Run
			if !buildOpen {
				openURL = func(input string) error { return nil }
			}

			config := cli.ImageBuildConfig{
				InitParameters:   initParams,
				RwxDirectory:     buildRwxDirectory,
				MintFilePath:     buildMintFilePath,
				NoCache:          buildNoCache,
				NoPull:           buildNoPull,
				TargetTaskKey:    buildTargetTaskKey,
				Tags:             buildTags,
				PushToReferences: buildPushToReferences,
				PushCompression:  buildPushCompression,
				Timeout:          buildTimeout,
				OpenURL:          openURL,
				OutputJSON:       useJsonOutput(),
			}

			_, err = getService().ImageBuild(config)
			return err
		},
		Short: "Launch a targeted RWX run and pull its result as an OCI image",
		Use:   "build <file> --target <task-key> [flags]",
	}

	BuildCmd.Flags().StringArrayVar(&buildInitParameters, "init", []string{}, "initialization parameters for the run, available in the `init` context. Can be specified multiple times")
	BuildCmd.Flags().StringVarP(&buildMintFilePath, "file", "f", "", "an RWX config file to use for sourcing task definitions (required)")
	BuildCmd.Flags().StringVarP(&buildRwxDirectory, "dir", "d", "", "the directory your RWX configuration files are located in, typically `.rwx`. By default, the CLI traverses up until it finds a `.rwx` directory.")
	BuildCmd.Flags().BoolVar(&buildNoCache, "no-cache", false, "do not read or write to the cache")
	BuildCmd.Flags().BoolVar(&buildNoPull, "no-pull", false, "do not pull the image after building")
	BuildCmd.Flags().StringVar(&buildTargetTaskKey, "target", "", "task key to build (required)")
	BuildCmd.Flags().StringArrayVar(&buildTags, "tag", []string{}, "tag the built image (can be specified multiple times)")
	BuildCmd.Flags().StringArrayVar(&buildPushToReferences, "push-to", []string{}, "push the built image to the specified OCI reference (can be specified multiple times)")
	BuildCmd.Flags().StringVar(&buildPushCompression, "push-compression", "zstd", "compression format for newly pushed layers in the image (zstd, gzip, none)")
	BuildCmd.Flags().DurationVar(&buildTimeout, "timeout", 30*time.Minute, "timeout for waiting for the build to complete and image to pull")
	BuildCmd.Flags().BoolVar(&buildOpen, "open", false, "open the build URL in the default browser once the build starts")

	_ = BuildCmd.MarkFlagRequired("target")
}
