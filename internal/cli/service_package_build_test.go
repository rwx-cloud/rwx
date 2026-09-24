package cli_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

// writePackageFixture creates a small package tree and returns its path.
func writePackageFixture(t *testing.T, dir string) string {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rwx-package.yml"), []byte("name: acme/thing\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".hidden"), []byte("hidden\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bin", "helper"), []byte("#!/bin/sh\n"), 0o755))

	return dir
}

// readZip returns the uploaded archive's entries keyed by name.
func readZip(t *testing.T, contents []byte) map[string]*zip.File {
	t.Helper()

	reader, err := zip.NewReader(bytes.NewReader(contents), int64(len(contents)))
	require.NoError(t, err)

	entries := make(map[string]*zip.File, len(reader.File))
	for _, f := range reader.File {
		entries[f.Name] = f
	}
	return entries
}

func TestService_BuildPrivatePackage(t *testing.T) {
	for _, filename := range []string{"rwx-package.yml", "mint-leaf.yml"} {
		for _, tc := range []struct {
			name     string
			manifest string
			want     string
		}{
			{
				name:     "adds legacy visibility",
				manifest: "# keep this comment\nname: acme/thing\n",
				want:     "name: acme/thing\nvisibility: private\n",
			},
			{
				name:     "overrides legacy visibility",
				manifest: "# keep this comment\nname: acme/thing\nvisibility: public\n",
				want:     "name: acme/thing\nvisibility: private\n",
			},
			{
				name:     "overrides wrapped visibility",
				manifest: "# keep this comment\npackage:\n  name: acme/thing\n  visibility: public\ntasks:\n  - key: hello\n    run: echo ${{ params.message }}\n",
				want:     "package:\n  name: acme/thing\n  visibility: private\ntasks:\n  - key: hello\n    run: echo ${{ params.message }}\n",
			},
		} {
			t.Run(filename+"/"+tc.name, func(t *testing.T) {
				s := setupTest(t)
				dir := t.TempDir()
				manifestPath := filepath.Join(dir, filename)
				require.NoError(t, os.WriteFile(manifestPath, []byte(tc.manifest), 0o644))
				before, err := os.Stat(manifestPath)
				require.NoError(t, err)

				var archives [][]byte
				s.mockAPI.MockUploadPackage = func(cfg api.UploadPackageConfig) (*api.UploadPackageResult, error) {
					data, err := io.ReadAll(cfg.Contents)
					require.NoError(t, err)
					archives = append(archives, data)
					return &api.UploadPackageResult{Digest: "private-digest"}, nil
				}
				for range 2 {
					result, err := s.service.BuildPackage(cli.PackageBuildConfig{
						Directory: dir,
						Private:   true,
						Timestamp: "202601020304",
					})
					require.NoError(t, err)
					require.Equal(t, "private-digest", result.Digest)
				}
				require.Equal(t, archives[0], archives[1])
				entry := readZip(t, archives[0])[filename]
				reader, err := entry.Open()
				require.NoError(t, err)
				defer reader.Close()
				contents, err := io.ReadAll(reader)
				require.NoError(t, err)
				require.YAMLEq(t, tc.want, string(contents))
				require.Contains(t, string(contents), "# keep this comment")
				require.Equal(t, time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC), entry.Modified.UTC())

				onDisk, err := os.ReadFile(manifestPath)
				require.NoError(t, err)
				require.Equal(t, tc.manifest, string(onDisk))
				after, err := os.Stat(manifestPath)
				require.NoError(t, err)
				require.Equal(t, before.ModTime(), after.ModTime())
			})
		}
	}
}

func TestService_BuildPackageManifestSelection(t *testing.T) {
	for _, private := range []bool{false, true} {
		t.Run(fmt.Sprintf("private=%t", private), func(t *testing.T) {
			s := setupTest(t)
			dir := t.TempDir()
			modern := "name: acme/modern\nvisibility: private\n"
			legacy := "name: acme/legacy\nvisibility: public\n"
			require.NoError(t, os.WriteFile(filepath.Join(dir, "rwx-package.yml"), []byte(modern), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "mint-leaf.yml"), []byte(legacy), 0o644))
			var uploaded []byte
			s.mockAPI.MockUploadPackage = func(cfg api.UploadPackageConfig) (*api.UploadPackageResult, error) {
				var err error
				uploaded, err = io.ReadAll(cfg.Contents)
				require.NoError(t, err)
				return &api.UploadPackageResult{Digest: "digest"}, nil
			}
			_, err := s.service.BuildPackage(cli.PackageBuildConfig{Directory: dir, Private: private})
			require.NoError(t, err)
			for name, entry := range readZip(t, uploaded) {
				reader, err := entry.Open()
				require.NoError(t, err)
				defer reader.Close()
				contents, err := io.ReadAll(reader)
				require.NoError(t, err)
				if name == "rwx-package.yml" {
					require.Equal(t, modern, string(contents), "only the manifest Cloud uses should be rewritten")
				} else if private {
					require.YAMLEq(t, "name: acme/legacy\nvisibility: private\n", string(contents))
				} else {
					require.Equal(t, legacy, string(contents))
				}
			}
		})
	}
}

func TestService_BuildPrivatePackageRejectsInvalidManifest(t *testing.T) {
	for _, manifest := range []string{"", "name: [", "- not-a-mapping\n", "package: true\n", "name: one\n---\nname: two\n"} {
		t.Run(manifest, func(t *testing.T) {
			s := setupTest(t)
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "mint-leaf.yml"), []byte(manifest), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "rwx-package.yml"), []byte("name: acme/valid\n"), 0o644))
			s.mockAPI.MockUploadPackage = func(cfg api.UploadPackageConfig) (*api.UploadPackageResult, error) {
				t.Fatal("must not upload or fall back to another manifest when --private cannot be applied")
				return nil, nil
			}
			_, err := s.service.BuildPackage(cli.PackageBuildConfig{Directory: dir, Private: true})
			require.Error(t, err)
		})
	}
	t.Run("missing manifest", func(t *testing.T) {
		s := setupTest(t)
		s.mockAPI.MockUploadPackage = func(cfg api.UploadPackageConfig) (*api.UploadPackageResult, error) {
			t.Fatal("must not upload without a private manifest")
			return nil, nil
		}
		_, err := s.service.BuildPackage(cli.PackageBuildConfig{Directory: t.TempDir(), Private: true})
		require.ErrorContains(t, err, "--private requires rwx-package.yml or mint-leaf.yml")
	})
}

func TestService_BuildPackage(t *testing.T) {
	t.Run("zips the directory contents at the archive root and reports the digest", func(t *testing.T) {
		s := setupTest(t)
		dir := writePackageFixture(t, filepath.Join(s.tmp, "pkg"))

		var uploaded []byte
		var uploadedName string
		s.mockAPI.MockUploadPackage = func(cfg api.UploadPackageConfig) (*api.UploadPackageResult, error) {
			var err error
			uploaded, err = io.ReadAll(cfg.Contents)
			require.NoError(t, err)
			uploadedName = cfg.FileName
			return &api.UploadPackageResult{Digest: "abc123"}, nil
		}

		result, err := s.service.BuildPackage(cli.PackageBuildConfig{Directory: dir})
		require.NoError(t, err)
		require.Equal(t, "abc123", result.Digest)
		require.Equal(t, "package.zip", uploadedName)
		require.Contains(t, s.mockStdout.String(), "Uploaded package with digest: abc123")

		entries := readZip(t, uploaded)
		require.Contains(t, entries, "rwx-package.yml")
		require.Contains(t, entries, ".hidden")
		require.Contains(t, entries, "bin/")
		require.Contains(t, entries, "bin/helper")
		// The directory name itself must not be part of the archive paths.
		require.NotContains(t, entries, "pkg/rwx-package.yml")

		body, err := entries["rwx-package.yml"].Open()
		require.NoError(t, err)
		defer body.Close()
		contents, err := io.ReadAll(body)
		require.NoError(t, err)
		require.Equal(t, "name: acme/thing\n", string(contents))

		require.Equal(t, os.FileMode(0o755), entries["bin/helper"].Mode().Perm(), "executable bit should be preserved")
	})

	t.Run("normalizes entry timestamps without touching the files on disk", func(t *testing.T) {
		s := setupTest(t)
		dir := writePackageFixture(t, filepath.Join(s.tmp, "pkg"))

		file := filepath.Join(dir, "rwx-package.yml")
		before, err := os.Stat(file)
		require.NoError(t, err)

		var uploaded []byte
		s.mockAPI.MockUploadPackage = func(cfg api.UploadPackageConfig) (*api.UploadPackageResult, error) {
			uploaded, err = io.ReadAll(cfg.Contents)
			require.NoError(t, err)
			return &api.UploadPackageResult{Digest: "abc123"}, nil
		}

		_, err = s.service.BuildPackage(cli.PackageBuildConfig{Directory: dir, Timestamp: "202601020304"})
		require.NoError(t, err)

		expected := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
		for name, entry := range readZip(t, uploaded) {
			require.Equal(t, expected, entry.Modified.UTC(), "entry %s should carry the normalized timestamp", name)
		}

		after, err := os.Stat(file)
		require.NoError(t, err)
		require.Equal(t, before.ModTime(), after.ModTime(), "source files must not be modified on disk")
	})

	t.Run("produces byte-identical archives for the same input and timestamp", func(t *testing.T) {
		s := setupTest(t)
		dir := writePackageFixture(t, filepath.Join(s.tmp, "pkg"))

		var archives [][]byte
		s.mockAPI.MockUploadPackage = func(cfg api.UploadPackageConfig) (*api.UploadPackageResult, error) {
			contents, err := io.ReadAll(cfg.Contents)
			require.NoError(t, err)
			archives = append(archives, contents)
			return &api.UploadPackageResult{Digest: "abc123"}, nil
		}

		for range 2 {
			_, err := s.service.BuildPackage(cli.PackageBuildConfig{Directory: dir, Timestamp: "202601020304"})
			require.NoError(t, err)
		}

		require.Len(t, archives, 2)
		require.Equal(t, archives[0], archives[1])
	})

	t.Run("preserves original modification times when no timestamp is given", func(t *testing.T) {
		s := setupTest(t)
		dir := writePackageFixture(t, filepath.Join(s.tmp, "pkg"))

		modTime := time.Date(2020, 5, 6, 7, 8, 0, 0, time.UTC)
		require.NoError(t, os.Chtimes(filepath.Join(dir, "rwx-package.yml"), modTime, modTime))

		var uploaded []byte
		s.mockAPI.MockUploadPackage = func(cfg api.UploadPackageConfig) (*api.UploadPackageResult, error) {
			var err error
			uploaded, err = io.ReadAll(cfg.Contents)
			require.NoError(t, err)
			return &api.UploadPackageResult{Digest: "abc123"}, nil
		}

		_, err := s.service.BuildPackage(cli.PackageBuildConfig{Directory: dir})
		require.NoError(t, err)

		// Zip stores timestamps at two-second granularity.
		actual := readZip(t, uploaded)["rwx-package.yml"].Modified.UTC()
		require.WithinDuration(t, modTime, actual, 2*time.Second)
	})

	t.Run("defaults to the current directory", func(t *testing.T) {
		s := setupTest(t)
		writePackageFixture(t, s.tmp)

		var uploaded []byte
		s.mockAPI.MockUploadPackage = func(cfg api.UploadPackageConfig) (*api.UploadPackageResult, error) {
			var err error
			uploaded, err = io.ReadAll(cfg.Contents)
			require.NoError(t, err)
			return &api.UploadPackageResult{Digest: "abc123"}, nil
		}

		_, err := s.service.BuildPackage(cli.PackageBuildConfig{})
		require.NoError(t, err)
		require.Contains(t, readZip(t, uploaded), "rwx-package.yml")
	})

	t.Run("writes JSON output when requested", func(t *testing.T) {
		s := setupTest(t)
		dir := writePackageFixture(t, filepath.Join(s.tmp, "pkg"))

		s.mockAPI.MockUploadPackage = func(cfg api.UploadPackageConfig) (*api.UploadPackageResult, error) {
			return &api.UploadPackageResult{Digest: "abc123"}, nil
		}

		_, err := s.service.BuildPackage(cli.PackageBuildConfig{Directory: dir, Json: true})
		require.NoError(t, err)

		var decoded struct{ Digest string }
		require.NoError(t, json.Unmarshal([]byte(s.mockStdout.String()), &decoded))
		require.Equal(t, "abc123", decoded.Digest)
	})

	t.Run("rejects a malformed timestamp before uploading", func(t *testing.T) {
		s := setupTest(t)
		dir := writePackageFixture(t, filepath.Join(s.tmp, "pkg"))

		s.mockAPI.MockUploadPackage = func(cfg api.UploadPackageConfig) (*api.UploadPackageResult, error) {
			t.Fatal("upload should not be attempted")
			return nil, nil
		}

		_, err := s.service.BuildPackage(cli.PackageBuildConfig{Directory: dir, Timestamp: "2026-01-02"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "YYYYMMDDHHmm")
	})

	t.Run("errors when the directory does not exist", func(t *testing.T) {
		s := setupTest(t)

		_, err := s.service.BuildPackage(cli.PackageBuildConfig{Directory: filepath.Join(s.tmp, "nope")})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unable to read")
	})

	t.Run("errors when the path is not a directory", func(t *testing.T) {
		s := setupTest(t)
		file := filepath.Join(s.tmp, "file.txt")
		require.NoError(t, os.WriteFile(file, []byte("hi"), 0o644))

		_, err := s.service.BuildPackage(cli.PackageBuildConfig{Directory: file})
		require.Error(t, err)
		require.Contains(t, err.Error(), "is not a directory")
	})

	t.Run("errors when the API returns no digest", func(t *testing.T) {
		s := setupTest(t)
		dir := writePackageFixture(t, filepath.Join(s.tmp, "pkg"))

		s.mockAPI.MockUploadPackage = func(cfg api.UploadPackageConfig) (*api.UploadPackageResult, error) {
			return &api.UploadPackageResult{}, nil
		}

		_, err := s.service.BuildPackage(cli.PackageBuildConfig{Directory: dir})
		require.Error(t, err)
		require.Contains(t, err.Error(), "did not return a package digest")
	})

	t.Run("records telemetry on success and failure", func(t *testing.T) {
		s := setupTest(t)
		dir := writePackageFixture(t, filepath.Join(s.tmp, "pkg"))

		s.mockAPI.MockUploadPackage = func(cfg api.UploadPackageConfig) (*api.UploadPackageResult, error) {
			return &api.UploadPackageResult{Digest: "abc123"}, nil
		}

		_, err := s.service.BuildPackage(cli.PackageBuildConfig{Directory: dir, Timestamp: "202601020304"})
		require.NoError(t, err)

		event := findEvent(s.drainEvents(), "package.build")
		require.NotNil(t, event)
		require.Equal(t, true, event.Props["success"])
		require.Equal(t, true, event.Props["normalized"])

		_, err = s.service.BuildPackage(cli.PackageBuildConfig{Directory: filepath.Join(s.tmp, "nope")})
		require.Error(t, err)

		event = findEvent(s.drainEvents(), "package.build")
		require.NotNil(t, event)
		require.Equal(t, false, event.Props["success"])
		require.Equal(t, false, event.Props["normalized"])
	})
}
