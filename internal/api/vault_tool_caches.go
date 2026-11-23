package api

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/rwx-cloud/rwx/internal/errors"
)

type VaultToolCache struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	InstallationID string     `json:"installation_id"`
	OS             string     `json:"os"`
	Arch           string     `json:"arch"`
	Populated      bool       `json:"populated"`
	LastReadAt     *time.Time `json:"last_read_at"`
	LastWrittenAt  *time.Time `json:"last_written_at"`
	LastEvictedAt  *time.Time `json:"last_evicted_at"`
	Expiry         *time.Time `json:"expiry"`
}

type ListVaultToolCachesConfig struct {
	VaultID string
}

type ListVaultToolCachesResult struct {
	ToolCaches []VaultToolCache `json:"tool_caches"`
}

type EvictVaultToolCacheConfig struct {
	VaultID     string
	ToolCacheID string
}

type EvictVaultToolCacheResult struct {
	ToolCache VaultToolCache `json:"tool_cache"`
}

func (c Client) ListVaultToolCaches(cfg ListVaultToolCachesConfig) (*ListVaultToolCachesResult, error) {
	result := &ListVaultToolCachesResult{ToolCaches: []VaultToolCache{}}
	cursor := ""
	for {
		endpoint := fmt.Sprintf("/mint/api/vaults/%s/tool_caches", url.PathEscape(cfg.VaultID))
		if cursor != "" {
			endpoint += "?" + url.Values{"cursor": {cursor}}.Encode()
		}
		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, errors.Wrap(err, "unable to create new HTTP request")
		}
		resp, err := c.RoundTrip(req)
		if err != nil {
			return nil, errors.Wrap(err, "HTTP request failed")
		}
		var page struct {
			ListVaultToolCachesResult
			Pagination struct {
				NextCursor string `json:"next_cursor"`
			} `json:"pagination"`
		}
		err = decodeResponseJSON(resp, &page)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		result.ToolCaches = append(result.ToolCaches, page.ToolCaches...)
		cursor = page.Pagination.NextCursor
		if cursor == "" {
			return result, nil
		}
	}
}

func (c Client) EvictVaultToolCache(cfg EvictVaultToolCacheConfig) (*EvictVaultToolCacheResult, error) {
	endpoint := fmt.Sprintf("/mint/api/vaults/%s/tool_caches/%s/evict", url.PathEscape(cfg.VaultID), url.PathEscape(cfg.ToolCacheID))
	req, err := http.NewRequest(http.MethodPost, endpoint, nil)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create new HTTP request")
	}
	resp, err := c.RoundTrip(req)
	if err != nil {
		return nil, errors.Wrap(err, "HTTP request failed")
	}
	defer resp.Body.Close()
	result := &EvictVaultToolCacheResult{}
	if err := decodeResponseJSON(resp, result); err != nil {
		return nil, err
	}
	return result, nil
}
