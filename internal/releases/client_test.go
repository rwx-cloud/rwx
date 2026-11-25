package releases

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func sha256Digest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func TestClient_Latest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/repos/rwx-cloud/rwx/releases/latest", r.URL.Path)
		require.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))

		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v3.1.0",
			"assets": []map[string]any{
				{"name": "rwx-linux-x86_64", "browser_download_url": "https://example.com/rwx-linux-x86_64", "digest": "sha256:abc"},
			},
		}))
	}))
	defer server.Close()

	c := Client{Host: server.Listener.Addr().String(), Scheme: "http"}
	release, err := c.Latest()

	require.NoError(t, err)
	require.Equal(t, "v3.1.0", release.TagName)

	asset, ok := release.Asset("rwx-linux-x86_64")
	require.True(t, ok)
	require.Equal(t, "sha256:abc", asset.Digest)

	_, ok = release.Asset("rwx-plan9-x86_64")
	require.False(t, ok)
}

func TestClient_Tag(t *testing.T) {
	t.Run("fetches the release for the tag", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "/repos/rwx-cloud/rwx/releases/tags/v3.0.0", r.URL.Path)
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"tag_name": "v3.0.0"}))
		}))
		defer server.Close()

		c := Client{Host: server.Listener.Addr().String(), Scheme: "http"}
		release, err := c.Tag("v3.0.0")

		require.NoError(t, err)
		require.Equal(t, "v3.0.0", release.TagName)
	})

	t.Run("when the tag does not exist", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		defer server.Close()

		c := Client{Host: server.Listener.Addr().String(), Scheme: "http"}
		_, err := c.Tag("v0.0.1")

		require.EqualError(t, err, "release not found")
	})
}

func TestClient_Download(t *testing.T) {
	const content = "#!/bin/sh\necho rwx\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	c := Client{}

	t.Run("writes the asset when the digest matches", func(t *testing.T) {
		var buf bytes.Buffer
		err := c.Download(Asset{Name: "rwx", BrowserDownloadURL: server.URL, Digest: sha256Digest(content)}, &buf)

		require.NoError(t, err)
		require.Equal(t, content, buf.String())
	})

	t.Run("fails when the digest does not match", func(t *testing.T) {
		var buf bytes.Buffer
		err := c.Download(Asset{Name: "rwx", BrowserDownloadURL: server.URL, Digest: sha256Digest("something else")}, &buf)

		require.ErrorContains(t, err, "checksum mismatch for rwx")
	})

	t.Run("fails when the asset has no digest", func(t *testing.T) {
		var buf bytes.Buffer
		err := c.Download(Asset{Name: "rwx", BrowserDownloadURL: server.URL}, &buf)

		require.EqualError(t, err, "release asset rwx has no SHA-256 digest to verify against")
	})
}
