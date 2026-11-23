package main

import (
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/spf13/cobra"
)

func init() {
	toolCachesCmd := &cobra.Command{
		Use:   "tool-caches",
		Short: "List and evict tool caches in a vault",
	}
	var listVault string
	listCmd := &cobra.Command{
		Use:   "list [flags]",
		Short: "List tool caches in a vault",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := service.ListVaultToolCaches(cli.ListVaultToolCachesConfig{Vault: listVault, Json: useJsonOutput()})
			return err
		},
	}
	listCmd.Flags().StringVar(&listVault, "vault", "", "the name or ID of the vault")
	_ = listCmd.MarkFlagRequired("vault")
	toolCachesCmd.AddCommand(listCmd)

	var evictVault string
	var yes bool
	evictCmd := &cobra.Command{
		Use:   "evict ID [flags]",
		Short: "Evict a tool cache from a vault",
		Args:  cobra.ExactArgs(1),
		Long:  "Invalidate a stored tool cache so the next successful task with write access can populate a fresh cache. Use an ID from tool-caches list.",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := service.EvictVaultToolCache(cli.EvictVaultToolCacheConfig{
				Vault:       evictVault,
				ToolCacheID: args[0],
				Json:        useJsonOutput(),
				Yes:         yes,
			})
			return err
		},
	}
	evictCmd.Flags().StringVar(&evictVault, "vault", "", "the name or ID of the vault")
	_ = evictCmd.MarkFlagRequired("vault")
	evictCmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")
	toolCachesCmd.AddCommand(evictCmd)
	vaultsCmd.AddCommand(toolCachesCmd)
}
