package imagearchive

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/rwx-cloud/rwx/internal/api"
)

// Write builds a Docker load archive without extracting downloaded layers.
func Write(ctx context.Context, output io.Writer, config *api.ImageConfiguration, imageRef string) error {
	tw := tar.NewWriter(output)
	image := v1.Image{Created: config.Created, Platform: v1.Platform{Architecture: config.Architecture, OS: config.OS},
		Config: v1.ImageConfig{Entrypoint: config.Entrypoint, Cmd: config.Command, User: config.User, WorkingDir: config.WorkingDir, Env: config.Env, Labels: map[string]string{}},
		RootFS: v1.RootFS{Type: "layers", DiffIDs: []digest.Digest{}},
	}
	for _, label := range config.Labels {
		image.Config.Labels[label.Key] = label.Value
	}
	layers := make([]string, 0, len(config.Layers))
	for i, layer := range config.Layers {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := fmt.Sprintf("%d/layer.tar", i)
		diffID, err := writeLayer(ctx, tw, name, layer)
		if err != nil {
			return fmt.Errorf("layer %s: %w", layer.ID, err)
		}
		layers = append(layers, name)
		image.RootFS.DiffIDs = append(image.RootFS.DiffIDs, diffID)
	}
	data, err := json.Marshal(image)
	if err != nil {
		return err
	}
	configName := fmt.Sprintf("%x.json", sha256.Sum256(data))
	if err := writeBytes(tw, configName, data); err != nil {
		return err
	}
	data, err = json.Marshal([]struct {
		Config   string
		RepoTags []string
		Layers   []string
	}{{configName, []string{imageRef}, layers}})
	if err != nil {
		return err
	}
	if err := writeBytes(tw, "manifest.json", data); err != nil {
		return err
	}
	return tw.Close()
}

func writeBytes(tw *tar.Writer, name string, data []byte) error {
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(data))}); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

func writeLayer(ctx context.Context, tw *tar.Writer, name string, layer api.ImageLayer) (digest.Digest, error) {
	manifestOnly := layer.Kind == "whiteout" && layer.SHA256 == ""
	f, err := os.CreateTemp("", "rwx-layer-*.tar")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	hash := sha256.New()
	out := io.MultiWriter(f, hash)
	if manifestOnly {
		err = writeWhiteout(ctx, out, layer.Entries)
	} else {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, layer.DownloadURL, nil)
		if reqErr != nil {
			return "", fmt.Errorf("invalid download URL")
		}
		// Presigned downloads must never receive the Cloud API bearer token.
		resp, downloadErr := http.DefaultClient.Do(req)
		if downloadErr != nil {
			// Client.Do wraps failures in url.Error, whose URL includes credentials.
			return "", fmt.Errorf("download failed: %w", errors.Unwrap(downloadErr))
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("download returned HTTP %d", resp.StatusCode)
		}
		var size int64
		size, err = io.Copy(out, resp.Body)
		if err == nil && size != layer.SizeInBytes {
			err = fmt.Errorf("archive size mismatch: got %d, expected %d", size, layer.SizeInBytes)
		}
	}
	if err != nil {
		return "", err
	}
	sum := fmt.Sprintf("%x", hash.Sum(nil))
	if !manifestOnly && sum != layer.SHA256 {
		return "", fmt.Errorf("archive SHA256 mismatch")
	}
	size, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return "", err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: size}); err != nil {
		return "", err
	}
	if _, err := io.Copy(tw, f); err != nil {
		return "", err
	}
	return digest.Digest("sha256:" + sum), ctx.Err()
}

func writeWhiteout(ctx context.Context, output io.Writer, entries []api.WhiteoutEntry) error {
	if entries == nil {
		return fmt.Errorf("whiteout entries are missing")
	}
	entries = slices.Clone(entries)
	slices.SortFunc(entries, func(a, b api.WhiteoutEntry) int { return strings.Compare(a.FullPath, b.FullPath) })
	tw := tar.NewWriter(output)
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := strings.TrimPrefix(entry.FullPath, "/")
		if name == "" || name == "." || path.IsAbs(name) || path.Clean(name) != name || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "\\") {
			return fmt.Errorf("invalid whiteout path %q", name)
		}
		header := &tar.Header{Name: name, Mode: entry.Mode, Uid: entry.UID, Gid: entry.GID, ModTime: entry.ModTime}
		switch {
		case entry.Type == 3 && entry.DevMajor == 0 && entry.DevMinor == 0:
			header.Name = path.Join(path.Dir(name), ".wh."+path.Base(name))
			header.Typeflag = tar.TypeReg
		case entry.Type == 5:
			header.Typeflag = tar.TypeDir
		default:
			return fmt.Errorf("unexpected entry in whiteout manifest: %q", name)
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if entry.Type == 5 && entry.IsOpaque {
			header.Name = path.Join(name, ".wh..wh..opq")
			header.Typeflag = tar.TypeReg
			if err := tw.WriteHeader(header); err != nil {
				return err
			}
		}
	}
	return tw.Close()
}
