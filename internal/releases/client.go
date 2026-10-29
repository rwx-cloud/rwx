package releases

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/rwx-cloud/rwx/cmd/rwx/config"
)

const repository = "rwx-cloud/rwx"

// Client fetches RWX CLI releases from the GitHub API.
type Client struct {
	Host   string // default "api.github.com"
	Scheme string // default "https"
}

type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
}

func (r Release) Asset(name string) (Asset, bool) {
	for _, asset := range r.Assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return Asset{}, false
}

func (c Client) host() string {
	if c.Host != "" {
		return c.Host
	}
	return "api.github.com"
}

func (c Client) scheme() string {
	if c.Scheme != "" {
		return c.Scheme
	}
	return "https"
}

// Latest returns the most recent stable release.
func (c Client) Latest() (*Release, error) {
	return c.fetchRelease(fmt.Sprintf("/repos/%s/releases/latest", repository))
}

// Tag returns the release for the given tag, e.g. "v3.1.0".
func (c Client) Tag(tag string) (*Release, error) {
	return c.fetchRelease(fmt.Sprintf("/repos/%s/releases/tags/%s", repository, url.PathEscape(tag)))
}

func (c Client) fetchRelease(path string) (*Release, error) {
	reqURL := fmt.Sprintf("%s://%s%s", c.scheme(), c.host(), path)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", fmt.Sprintf("rwx-cli/%s", config.Version))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("release not found")
		}
		return nil, fmt.Errorf("GitHub releases returned status %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("unable to parse release response: %w", err)
	}

	return &release, nil
}

// Download writes the asset to dst and verifies it against the SHA-256 digest
// GitHub reports for it. Assets without a digest are rejected.
func (c Client) Download(asset Asset, dst io.Writer) error {
	expected, ok := strings.CutPrefix(asset.Digest, "sha256:")
	if !ok {
		return fmt.Errorf("release asset %s has no SHA-256 digest to verify against", asset.Name)
	}

	req, err := http.NewRequest("GET", asset.BrowserDownloadURL, nil)
	if err != nil {
		return fmt.Errorf("unable to create request: %w", err)
	}
	req.Header.Set("User-Agent", fmt.Sprintf("rwx-cli/%s", config.Version))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("unable to download %s: %w", asset.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s returned status %d", asset.Name, resp.StatusCode)
	}

	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(dst, hash), resp.Body); err != nil {
		return fmt.Errorf("unable to download %s: %w", asset.Name, err)
	}

	if actual := hex.EncodeToString(hash.Sum(nil)); actual != expected {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", asset.Name, expected, actual)
	}

	return nil
}
