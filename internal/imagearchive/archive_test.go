package imagearchive_test

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/imagearchive"
	"github.com/stretchr/testify/require"
)

func TestWrite(t *testing.T) {
	var archived bytes.Buffer
	tw := tar.NewWriter(&archived)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: ".wh.old", Mode: 0600}))
	require.NoError(t, tw.Close())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Empty(t, r.Header.Get("Authorization"))
		_, _ = w.Write(archived.Bytes())
	}))
	defer server.Close()
	created := time.Date(2026, 10, 31, 12, 34, 56, 0, time.UTC)
	cfg := &api.ImageConfiguration{Architecture: "arm64", OS: "linux", Created: &created, Entrypoint: []string{"/entry"}, Command: []string{"arg"}, User: "123:456", WorkingDir: "/work", Env: []string{"A=one", "B=two"}}
	require.NoError(t, json.Unmarshal([]byte(`{"labels":[{"key":"example","value":"yes"}]}`), cfg))
	old := api.ImageLayer{ID: "archived", SHA256: fmt.Sprintf("%x", sha256.Sum256(archived.Bytes())), SizeInBytes: int64(archived.Len()), DownloadURL: server.URL}
	cfg.Layers = []api.ImageLayer{old, {ID: "whiteout", Kind: "whiteout", Entries: []api.WhiteoutEntry{
		{FullPath: "/dir/deleted", Type: 3, Mode: 0600, UID: 12, GID: 34, ModTime: created},
		{FullPath: "/dir", Type: 5, Mode: 0751, UID: 56, GID: 78, ModTime: created, IsOpaque: true},
	}}, old}
	var out bytes.Buffer
	require.NoError(t, imagearchive.Write(context.Background(), &out, cfg, "example.test/org:task"))
	files := readArchive(t, out.Bytes())
	var manifest []struct {
		Config           string
		RepoTags, Layers []string
	}
	require.NoError(t, json.Unmarshal(files["manifest.json"], &manifest))
	require.Len(t, manifest, 1)
	require.Equal(t, []string{"example.test/org:task"}, manifest[0].RepoTags)
	require.Equal(t, []string{"0/layer.tar", "1/layer.tar", "2/layer.tar"}, manifest[0].Layers)
	require.Equal(t, archived.Bytes(), files["0/layer.tar"])
	require.Equal(t, archived.Bytes(), files["2/layer.tar"])
	// Shared fixture generated independently by images-cli generate-whiteout.
	require.Len(t, files["1/layer.tar"], 2560)
	require.Equal(t, "c2457e01131baf20d6f19bf9073c3b3a0d8b8dea2466dc8c7261f85814555a67", fmt.Sprintf("%x", sha256.Sum256(files["1/layer.tar"])))
	var image v1.Image
	require.NoError(t, json.Unmarshal(files[manifest[0].Config], &image))
	require.Equal(t, "arm64", image.Architecture)
	require.Equal(t, "linux", image.OS)
	require.Equal(t, &created, image.Created)
	require.Equal(t, cfg.Entrypoint, image.Config.Entrypoint)
	require.Equal(t, cfg.Command, image.Config.Cmd)
	require.Equal(t, cfg.Env, image.Config.Env)
	require.Equal(t, cfg.User, image.Config.User)
	require.Equal(t, cfg.WorkingDir, image.Config.WorkingDir)
	require.Equal(t, map[string]string{"example": "yes"}, image.Config.Labels)
	for i, name := range manifest[0].Layers {
		require.Equal(t, fmt.Sprintf("sha256:%x", sha256.Sum256(files[name])), string(image.RootFS.DiffIDs[i]))
	}
	tr := tar.NewReader(bytes.NewReader(files["1/layer.tar"]))
	h, err := tr.Next()
	require.NoError(t, err)
	require.Equal(t, "dir", h.Name)
	require.Equal(t, byte(tar.TypeDir), h.Typeflag)
	require.Equal(t, int64(0751), h.Mode)
	require.Equal(t, 56, h.Uid)
	require.Equal(t, 78, h.Gid)
	require.True(t, created.Equal(h.ModTime))
	h, err = tr.Next()
	require.NoError(t, err)
	require.Equal(t, "dir/.wh..wh..opq", h.Name)
	require.Equal(t, byte(tar.TypeReg), h.Typeflag)
	h, err = tr.Next()
	require.NoError(t, err)
	require.Equal(t, "dir/.wh.deleted", h.Name)
	require.Equal(t, byte(tar.TypeReg), h.Typeflag)
	require.Zero(t, h.Size)
	_, err = tr.Next()
	require.ErrorIs(t, err, io.EOF)
	var again bytes.Buffer
	require.NoError(t, imagearchive.Write(context.Background(), &again, cfg, "example.test/org:task"))
	require.Equal(t, out.Bytes(), again.Bytes())
	for _, tc := range []struct {
		name  string
		layer api.ImageLayer
		want  string
	}{
		{"missing entries", api.ImageLayer{Kind: "whiteout"}, "entries are missing"},
		{"traversal", api.ImageLayer{Kind: "whiteout", Entries: []api.WhiteoutEntry{{FullPath: "/../escape", Type: 3}}}, "invalid whiteout path"},
		{"nonwhiteout", api.ImageLayer{Kind: "whiteout", Entries: []api.WhiteoutEntry{{FullPath: "/device", Type: 3, DevMajor: 1}}}, "unexpected entry"},
		{"digest", api.ImageLayer{SHA256: "bad", SizeInBytes: old.SizeInBytes, DownloadURL: server.URL}, "SHA256 mismatch"},
		{"size", api.ImageLayer{SHA256: old.SHA256, SizeInBytes: old.SizeInBytes + 1, DownloadURL: server.URL}, "size mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.ErrorContains(t, imagearchive.Write(context.Background(), io.Discard, &api.ImageConfiguration{Layers: []api.ImageLayer{tc.layer}}, "test:tag"), tc.want)
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, imagearchive.Write(ctx, io.Discard, cfg, "test:tag"), context.Canceled)
}

func readArchive(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	tr := tar.NewReader(bytes.NewReader(data))
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		files[h.Name], err = io.ReadAll(tr)
		require.NoError(t, err)
	}
	return files
}
