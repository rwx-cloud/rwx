package cli

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

// PackageTimestampLayout is the accepted format for --timestamp (YYYYMMDDHHmm).
const PackageTimestampLayout = "200601021504"

type PackageBuildConfig struct {
	Directory string
	Timestamp string
	Json      bool
}

type PackageBuildResult struct {
	Digest string
}

// BuildPackage zips the contents of a directory and uploads it to the RWX
// package registry, returning the digest assigned by the server.
func (s Service) BuildPackage(cfg PackageBuildConfig) (result *PackageBuildResult, err error) {
	start := time.Now()
	defer func() {
		s.recordTelemetry("package.build", map[string]any{
			"duration_ms": time.Since(start).Milliseconds(),
			"normalized":  cfg.Timestamp != "",
			"success":     err == nil,
		})
	}()

	directory := cfg.Directory
	if directory == "" {
		directory = "."
	}

	// An empty timestamp leaves each file's own modification time in place.
	var modTime *time.Time
	if cfg.Timestamp != "" {
		parsed, parseErr := time.ParseInLocation(PackageTimestampLayout, cfg.Timestamp, time.UTC)
		if parseErr != nil {
			return nil, errors.Wrapf(parseErr, "invalid timestamp %q; expected format YYYYMMDDHHmm", cfg.Timestamp)
		}
		modTime = &parsed
	}

	info, err := os.Stat(directory)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to read %q", directory)
	}
	if !info.IsDir() {
		return nil, errors.Errorf("%q is not a directory", directory)
	}

	archive, err := zipDirectory(directory, modTime)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to zip %q", directory)
	}

	uploaded, err := s.APIClient.UploadPackage(api.UploadPackageConfig{
		FileName: "package.zip",
		Contents: bytes.NewReader(archive),
	})
	if err != nil {
		return nil, errors.Wrap(err, "unable to upload package")
	}
	if uploaded.Digest == "" {
		return nil, errors.New("the RWX API did not return a package digest")
	}

	result = &PackageBuildResult{Digest: uploaded.Digest}

	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		fmt.Fprintf(s.Stdout, "Uploaded package with digest: %s\n", result.Digest)
	}

	return result, nil
}

// zipDirectory recursively zips the contents of root, placing them at the root
// of the archive. Entries are written in a stable order and, when modTime is
// non-nil, every entry's modification time is normalized in the zip header
// rather than on disk, so the archive is reproducible without mutating the
// source tree.
func zipDirectory(root string, modTime *time.Time) ([]byte, error) {
	var relPaths []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		relPaths = append(relPaths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(relPaths)

	buf := new(bytes.Buffer)
	archive := zip.NewWriter(buf)

	for _, rel := range relPaths {
		path := filepath.Join(root, filepath.FromSlash(rel))

		// Stat rather than Lstat so symlinks are followed and stored as their
		// target, matching `zip -r` without `-y`.
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			continue
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return nil, err
		}
		header.Name = rel
		if info.IsDir() {
			header.Name += "/"
			header.Method = zip.Store
		} else {
			header.Method = zip.Deflate
		}
		if modTime != nil {
			header.Modified = *modTime
		}

		entry, err := archive.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			continue
		}

		if err := copyFileInto(entry, path); err != nil {
			return nil, err
		}
	}

	if err := archive.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func copyFileInto(w io.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(w, file)
	return err
}
