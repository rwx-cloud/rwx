package api

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

type ImageConfiguration struct {
	Architecture string     `json:"architecture"`
	OS           string     `json:"os"`
	Created      *time.Time `json:"created"`
	Entrypoint   []string   `json:"entrypoint"`
	Command      []string   `json:"command"`
	User         string     `json:"user"`
	WorkingDir   string     `json:"working_dir"`
	Env          []string   `json:"env"`
	Labels       []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"labels"`
	Layers []ImageLayer `json:"layers"`
}

type ImageLayer struct {
	ID                 string          `json:"id"`
	Kind               string          `json:"kind"`
	SHA256             string          `json:"sha256"`
	SizeInBytes        int64           `json:"size_in_bytes"`
	DownloadURL        string          `json:"download_url"`
	ManifestID         string          `json:"manifest_id"`
	ManifestCLIVersion string          `json:"manifest_cli_version"`
	Entries            []WhiteoutEntry `json:"entries"`
}

type WhiteoutEntry struct {
	FullPath string    `json:"fullPath"`
	Type     int       `json:"type"`
	Mode     int64     `json:"mode"`
	UID      int       `json:"uid"`
	GID      int       `json:"gid"`
	DevMajor int64     `json:"devmajor"`
	DevMinor int64     `json:"devminor"`
	ModTime  time.Time `json:"modTime"`
	IsOpaque bool      `json:"isOpaque"`
}

func (c Client) ImagePullConfiguration(ctx context.Context, taskID string) (*ImageConfiguration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/mint/api/images/pulls/"+url.PathEscape(taskID)+"/configuration", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result ImageConfiguration
	if err := decodeResponseJSON(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
