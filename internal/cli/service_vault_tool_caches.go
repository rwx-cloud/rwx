package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type VaultToolCacheInfo struct {
	ID             string
	Name           string
	InstallationID string
	OS             string
	Arch           string
	Populated      bool
	LastReadAt     *time.Time
	LastWrittenAt  *time.Time
	LastEvictedAt  *time.Time
	Expiry         *time.Time
}

type ListVaultToolCachesConfig struct {
	Vault string
	Json  bool
}

type ListVaultToolCachesResult struct {
	ToolCaches []VaultToolCacheInfo
}

type EvictVaultToolCacheConfig struct {
	Vault       string
	ToolCacheID string
	Json        bool
	Yes         bool
}

type EvictVaultToolCacheResult = VaultToolCacheInfo

func vaultToolCacheInfo(cache api.VaultToolCache) VaultToolCacheInfo {
	return VaultToolCacheInfo{
		ID:             cache.ID,
		Name:           cache.Name,
		InstallationID: cache.InstallationID,
		OS:             cache.OS,
		Arch:           cache.Arch,
		Populated:      cache.Populated,
		LastReadAt:     cache.LastReadAt,
		LastWrittenAt:  cache.LastWrittenAt,
		LastEvictedAt:  cache.LastEvictedAt,
		Expiry:         cache.Expiry,
	}
}

func (s Service) ListVaultToolCaches(cfg ListVaultToolCachesConfig) (*ListVaultToolCachesResult, error) {
	vaultID, err := s.resolveVaultID(cfg.Vault)
	if err != nil {
		return nil, err
	}
	listed, err := s.APIClient.ListVaultToolCaches(api.ListVaultToolCachesConfig{VaultID: vaultID})
	if err != nil {
		return nil, errors.Wrap(err, "unable to list tool caches")
	}
	result := &ListVaultToolCachesResult{ToolCaches: make([]VaultToolCacheInfo, len(listed.ToolCaches))}
	for i, cache := range listed.ToolCaches {
		result.ToolCaches[i] = vaultToolCacheInfo(cache)
	}
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else if len(result.ToolCaches) == 0 {
		fmt.Fprintf(s.Stdout, "No tool caches found in vault %q.\n", cfg.Vault)
	} else {
		w := tabwriter.NewWriter(s.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tINSTALLATION ID\tOS\tARCH\tPOPULATED")
		for _, cache := range result.ToolCaches {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%t\n", cache.ID, cache.Name, cache.InstallationID, cache.OS, cache.Arch, cache.Populated)
		}
		if err := w.Flush(); err != nil {
			return nil, errors.Wrap(err, "unable to write tool cache list")
		}
	}
	return result, nil
}

func (s Service) EvictVaultToolCache(cfg EvictVaultToolCacheConfig) (*EvictVaultToolCacheResult, error) {
	if cfg.ToolCacheID == "" {
		return nil, errors.New("the tool cache ID must be provided")
	}
	vaultID, err := s.resolveVaultID(cfg.Vault)
	if err != nil {
		return nil, err
	}
	prompt := fmt.Sprintf("Evict tool cache %q from vault %q?", cfg.ToolCacheID, cfg.Vault)
	if err := s.confirmDestruction(prompt, cfg.Yes); err != nil {
		return nil, err
	}
	evicted, err := s.APIClient.EvictVaultToolCache(api.EvictVaultToolCacheConfig{VaultID: vaultID, ToolCacheID: cfg.ToolCacheID})
	if err != nil {
		return nil, errors.Wrap(err, "unable to evict tool cache")
	}
	result := vaultToolCacheInfo(evicted.ToolCache)
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		fmt.Fprintf(s.Stdout, "Evicted tool cache %q (%s) from vault %q.\n", result.Name, result.ID, cfg.Vault)
	}
	return &result, nil
}
