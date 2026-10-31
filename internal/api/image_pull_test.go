package api_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/stretchr/testify/require"
)

func TestImagePullConfiguration(t *testing.T) {
	c := api.NewClientWithRoundTrip(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "/mint/api/images/pulls/task-id/configuration", r.URL.Path)
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "application/json", r.Header.Get("Accept"))
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"architecture":"amd64","os":"linux","created":"2026-10-31T12:34:56Z","entrypoint":null,"command":["/bin/bash"],"user":null,"working_dir":null,"env":null,"labels":[],"layers":[{"id":"old","sha256":"abc","size_in_bytes":512,"download_url":"https://storage.test/archive"},{"id":"new","kind":"whiteout","manifest_id":"manifest","manifest_cli_version":"version","entries":[{"fullPath":"/gone","type":3,"mode":384,"uid":12,"gid":34,"devmajor":0,"devminor":0,"modTime":"2026-10-31T12:34:56Z","isOpaque":false}]}]}`))}, nil
	})
	cfg, err := c.ImagePullConfiguration(context.Background(), "task-id")
	require.NoError(t, err)
	require.Equal(t, "amd64", cfg.Architecture)
	require.Equal(t, "2026-10-31T12:34:56Z", cfg.Created.Format("2006-01-02T15:04:05Z07:00"))
	require.Len(t, cfg.Layers, 2)
	require.Empty(t, cfg.Layers[0].Kind)
	require.Equal(t, int64(512), cfg.Layers[0].SizeInBytes)
	require.Equal(t, "whiteout", cfg.Layers[1].Kind)
	require.Empty(t, cfg.Layers[1].SHA256)
	require.Equal(t, "/gone", cfg.Layers[1].Entries[0].FullPath)
	require.Equal(t, 12, cfg.Layers[1].Entries[0].UID)
	require.False(t, cfg.Layers[1].Entries[0].ModTime.IsZero())
}
