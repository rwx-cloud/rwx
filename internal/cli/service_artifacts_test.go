package cli_test

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestService_DownloadArtifact(t *testing.T) {
	t.Run("when the artifact is not found", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockGetArtifactDownloadRequest = func(taskId, artifactKey string) (api.ArtifactDownloadRequestResult, error) {
			require.Equal(t, "task-123", taskId)
			require.Equal(t, "my-artifact", artifactKey)
			return api.ArtifactDownloadRequestResult{}, api.ErrNotFound
		}

		_, err := s.service.DownloadArtifact(cli.DownloadArtifactConfig{
			TaskID:      "task-123",
			ArtifactKey: "my-artifact",
			OutputDir:   s.tmp,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "Artifact my-artifact for task task-123 not found")
	})

	t.Run("when GetArtifactDownloadRequest fails with other error", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockGetArtifactDownloadRequest = func(taskId, artifactKey string) (api.ArtifactDownloadRequestResult, error) {
			return api.ArtifactDownloadRequestResult{}, errors.New("network error")
		}

		_, err := s.service.DownloadArtifact(cli.DownloadArtifactConfig{
			TaskID:      "task-123",
			ArtifactKey: "my-artifact",
			OutputDir:   s.tmp,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "unable to fetch artifact download request")
		require.Contains(t, err.Error(), "network error")
	})

	t.Run("when DownloadArtifact fails", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockGetArtifactDownloadRequest = func(taskId, artifactKey string) (api.ArtifactDownloadRequestResult, error) {
			return api.ArtifactDownloadRequestResult{
				URL:      "https://example.com/artifact",
				Filename: "task-123-my-artifact.tar",
				Kind:     "file",
				Key:      "my-artifact",
			}, nil
		}

		s.mockAPI.MockDownloadArtifact = func(request api.ArtifactDownloadRequestResult) ([]byte, error) {
			require.Equal(t, "https://example.com/artifact", request.URL)
			require.Equal(t, "task-123-my-artifact.tar", request.Filename)
			return nil, errors.New("download failed")
		}

		_, err := s.service.DownloadArtifact(cli.DownloadArtifactConfig{
			TaskID:      "task-123",
			ArtifactKey: "my-artifact",
			OutputDir:   s.tmp,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "unable to download artifact")
		require.Contains(t, err.Error(), "download failed")
		require.Contains(t, s.mockStderr.String(), "Downloading artifact...")
	})

	t.Run("when validation fails - missing task ID", func(t *testing.T) {
		s := setupTest(t)

		_, err := s.service.DownloadArtifact(cli.DownloadArtifactConfig{
			TaskID:      "",
			ArtifactKey: "my-artifact",
			OutputDir:   s.tmp,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "validation failed")
		require.Contains(t, err.Error(), "task ID must be provided")
	})

	t.Run("when validation fails - missing artifact key", func(t *testing.T) {
		s := setupTest(t)

		_, err := s.service.DownloadArtifact(cli.DownloadArtifactConfig{
			TaskID:      "task-123",
			ArtifactKey: "",
			OutputDir:   s.tmp,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "validation failed")
		require.Contains(t, err.Error(), "artifact key must be provided")
	})

	t.Run("when validation fails - missing output directory", func(t *testing.T) {
		s := setupTest(t)

		_, err := s.service.DownloadArtifact(cli.DownloadArtifactConfig{
			TaskID:      "task-123",
			ArtifactKey: "my-artifact",
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "validation failed")
		require.Contains(t, err.Error(), "output directory must be provided")
	})

	t.Run("when download succeeds with file artifact - always extracts", func(t *testing.T) {
		s := setupTest(t)

		fileContent := []byte("artifact file content")
		tarBytes := createTestTar(t, map[string][]byte{
			"myfile.txt": fileContent,
		})

		s.mockAPI.MockGetArtifactDownloadRequest = func(taskId, artifactKey string) (api.ArtifactDownloadRequestResult, error) {
			require.Equal(t, "task-123", taskId)
			require.Equal(t, "my-file", artifactKey)
			return api.ArtifactDownloadRequestResult{
				URL:      "https://example.com/artifact",
				Filename: "task-123-my-file.tar",
				Kind:     "file",
				Key:      "my-file",
			}, nil
		}

		s.mockAPI.MockDownloadArtifact = func(request api.ArtifactDownloadRequestResult) ([]byte, error) {
			return tarBytes, nil
		}

		_, err := s.service.DownloadArtifact(cli.DownloadArtifactConfig{
			TaskID:      "task-123",
			ArtifactKey: "my-file",
			OutputDir:   s.tmp,
			AutoExtract: false, // Should extract anyway for files
		})

		require.NoError(t, err)
		extractDir := filepath.Join(s.tmp, "task-123-my-file")
		expectedPath := filepath.Join(extractDir, "myfile.txt")
		require.FileExists(t, expectedPath)

		actualContents, err := os.ReadFile(expectedPath)
		require.NoError(t, err)
		require.Equal(t, fileContent, actualContents)

		output := s.mockStdout.String()
		require.Contains(t, output, "Artifact downloaded to")
		require.Contains(t, output, "myfile.txt")
		require.Contains(t, s.mockStderr.String(), "Downloading artifact...")
	})

	t.Run("when download succeeds with directory artifact and auto-extract false - saves tar", func(t *testing.T) {
		s := setupTest(t)

		tarBytes := createTestTar(t, map[string][]byte{
			"file1.txt": []byte("content 1"),
			"file2.txt": []byte("content 2"),
		})

		s.mockAPI.MockGetArtifactDownloadRequest = func(taskId, artifactKey string) (api.ArtifactDownloadRequestResult, error) {
			return api.ArtifactDownloadRequestResult{
				URL:      "https://example.com/artifact",
				Filename: "task-456-my-dir.tar",
				Kind:     "directory",
				Key:      "my-dir",
			}, nil
		}

		s.mockAPI.MockDownloadArtifact = func(request api.ArtifactDownloadRequestResult) ([]byte, error) {
			return tarBytes, nil
		}

		_, err := s.service.DownloadArtifact(cli.DownloadArtifactConfig{
			TaskID:      "task-456",
			ArtifactKey: "my-dir",
			OutputDir:   s.tmp,
			AutoExtract: false,
		})

		require.NoError(t, err)
		expectedPath := filepath.Join(s.tmp, "task-456-my-dir.tar")
		require.FileExists(t, expectedPath)

		actualContents, err := os.ReadFile(expectedPath)
		require.NoError(t, err)
		require.Equal(t, tarBytes, actualContents)

		output := s.mockStdout.String()
		require.Contains(t, output, "Artifact downloaded to")
		require.Contains(t, output, "task-456-my-dir.tar")
	})

	t.Run("when download succeeds with directory artifact and auto-extract true - extracts", func(t *testing.T) {
		s := setupTest(t)

		tarBytes := createTestTar(t, map[string][]byte{
			"file1.txt":        []byte("content 1"),
			"file2.txt":        []byte("content 2"),
			"subdir/file3.txt": []byte("content 3"),
		})

		s.mockAPI.MockGetArtifactDownloadRequest = func(taskId, artifactKey string) (api.ArtifactDownloadRequestResult, error) {
			return api.ArtifactDownloadRequestResult{
				URL:      "https://example.com/artifact",
				Filename: "task-789-my-dir.tar",
				Kind:     "directory",
				Key:      "my-dir",
			}, nil
		}

		s.mockAPI.MockDownloadArtifact = func(request api.ArtifactDownloadRequestResult) ([]byte, error) {
			return tarBytes, nil
		}

		_, err := s.service.DownloadArtifact(cli.DownloadArtifactConfig{
			TaskID:      "task-789",
			ArtifactKey: "my-dir",
			OutputDir:   s.tmp,
			AutoExtract: true,
		})

		require.NoError(t, err)
		extractDir := filepath.Join(s.tmp, "task-789-my-dir")
		require.FileExists(t, filepath.Join(extractDir, "file1.txt"))
		require.FileExists(t, filepath.Join(extractDir, "file2.txt"))
		require.FileExists(t, filepath.Join(extractDir, "subdir", "file3.txt"))

		content1, err := os.ReadFile(filepath.Join(extractDir, "file1.txt"))
		require.NoError(t, err)
		require.Equal(t, []byte("content 1"), content1)

		output := s.mockStdout.String()
		require.Contains(t, output, "Extracted 3 file(s)")
		require.Contains(t, output, "file1.txt")
		require.Contains(t, output, "file2.txt")
		require.Contains(t, output, "subdir/file3.txt")
	})

	t.Run("when download succeeds with explicit output directory for file artifact", func(t *testing.T) {
		s := setupTest(t)

		fileContent := []byte("custom file content")
		tarBytes := createTestTar(t, map[string][]byte{
			"original.txt": fileContent,
		})

		customOutputDir := filepath.Join(s.tmp, "custom")
		s.mockAPI.MockGetArtifactDownloadRequest = func(taskId, artifactKey string) (api.ArtifactDownloadRequestResult, error) {
			return api.ArtifactDownloadRequestResult{
				URL:      "https://example.com/artifact",
				Filename: "task-999-my-file.tar",
				Kind:     "file",
				Key:      "my-file",
			}, nil
		}

		s.mockAPI.MockDownloadArtifact = func(request api.ArtifactDownloadRequestResult) ([]byte, error) {
			return tarBytes, nil
		}

		_, err := s.service.DownloadArtifact(cli.DownloadArtifactConfig{
			TaskID:                 "task-999",
			ArtifactKey:            "my-file",
			OutputDir:              customOutputDir,
			OutputDirExplicitlySet: true,
		})

		require.NoError(t, err)
		expectedPath := filepath.Join(customOutputDir, "original.txt")
		require.FileExists(t, expectedPath)
		require.NoDirExists(t, filepath.Join(customOutputDir, "task-999-my-file"))

		actualContents, err := os.ReadFile(expectedPath)
		require.NoError(t, err)
		require.Equal(t, fileContent, actualContents)

		output := s.mockStdout.String()
		require.Contains(t, output, "Artifact downloaded to")
		require.Contains(t, output, "original.txt")
	})

	t.Run("when auto-extracting directory artifact with explicit output directory, extracts directly into it", func(t *testing.T) {
		s := setupTest(t)

		tarBytes := createTestTar(t, map[string][]byte{
			"file1.txt":        []byte("content 1"),
			"subdir/file2.txt": []byte("content 2"),
		})

		s.mockAPI.MockGetArtifactDownloadRequest = func(taskId, artifactKey string) (api.ArtifactDownloadRequestResult, error) {
			return api.ArtifactDownloadRequestResult{
				URL:      "https://example.com/artifact",
				Filename: "task-333-my-dir.tar",
				Kind:     "directory",
				Key:      "my-dir",
			}, nil
		}

		s.mockAPI.MockDownloadArtifact = func(request api.ArtifactDownloadRequestResult) ([]byte, error) {
			return tarBytes, nil
		}

		outputDir := filepath.Join(s.tmp, "requested-output")
		result, err := s.service.DownloadArtifact(cli.DownloadArtifactConfig{
			TaskID:                 "task-333",
			ArtifactKey:            "my-dir",
			OutputDir:              outputDir,
			OutputDirExplicitlySet: true,
			AutoExtract:            true,
		})

		require.NoError(t, err)
		require.Len(t, result.OutputFiles, 2)
		require.FileExists(t, filepath.Join(outputDir, "file1.txt"))
		require.FileExists(t, filepath.Join(outputDir, "subdir", "file2.txt"))
		require.NoDirExists(t, filepath.Join(outputDir, "task-333-my-dir"))

		output := s.mockStdout.String()
		require.Contains(t, output, "Extracted 2 file(s)")
		require.Contains(t, output, outputDir)
	})

	t.Run("when download succeeds with JSON output - single file", func(t *testing.T) {
		s := setupTest(t)

		tarBytes := createTestTar(t, map[string][]byte{
			"result.json": []byte(`{"status":"success"}`),
		})

		s.mockAPI.MockGetArtifactDownloadRequest = func(taskId, artifactKey string) (api.ArtifactDownloadRequestResult, error) {
			return api.ArtifactDownloadRequestResult{
				URL:      "https://example.com/artifact",
				Filename: "task-111-result.tar",
				Kind:     "file",
				Key:      "result",
			}, nil
		}

		s.mockAPI.MockDownloadArtifact = func(request api.ArtifactDownloadRequestResult) ([]byte, error) {
			return tarBytes, nil
		}

		_, err := s.service.DownloadArtifact(cli.DownloadArtifactConfig{
			TaskID:      "task-111",
			ArtifactKey: "result",
			OutputDir:   s.tmp,
			Json:        true,
		})

		require.NoError(t, err)
		output := s.mockStdout.String()
		require.Contains(t, output, `"OutputFiles"`)
		require.Contains(t, output, "result.json")
		require.NotContains(t, output, "Artifact downloaded to")
	})

	t.Run("when download succeeds with JSON output and auto-extract - directory", func(t *testing.T) {
		s := setupTest(t)

		tarBytes := createTestTar(t, map[string][]byte{
			"file1.txt": []byte("content 1"),
			"file2.txt": []byte("content 2"),
		})

		s.mockAPI.MockGetArtifactDownloadRequest = func(taskId, artifactKey string) (api.ArtifactDownloadRequestResult, error) {
			return api.ArtifactDownloadRequestResult{
				URL:      "https://example.com/artifact",
				Filename: "task-222-my-dir.tar",
				Kind:     "directory",
				Key:      "my-dir",
			}, nil
		}

		s.mockAPI.MockDownloadArtifact = func(request api.ArtifactDownloadRequestResult) ([]byte, error) {
			return tarBytes, nil
		}

		_, err := s.service.DownloadArtifact(cli.DownloadArtifactConfig{
			TaskID:      "task-222",
			ArtifactKey: "my-dir",
			OutputDir:   s.tmp,
			AutoExtract: true,
			Json:        true,
		})

		require.NoError(t, err)
		output := s.mockStdout.String()
		require.Contains(t, output, `"OutputFiles"`)
		require.Contains(t, output, "file1.txt")
		require.Contains(t, output, "file2.txt")
		require.NotContains(t, output, "Extracted")
		require.NotContains(t, output, "Artifact downloaded")
	})

	t.Run("when tar contains ./ directory entry", func(t *testing.T) {
		s := setupTest(t)

		// Create tar with ./ entry (common in some tar files)
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)

		// Add ./ directory entry
		err := tw.WriteHeader(&tar.Header{
			Name:     "./",
			Typeflag: tar.TypeDir,
			Mode:     0755,
		})
		require.NoError(t, err)

		// Add a regular file
		content := []byte("file content")
		err = tw.WriteHeader(&tar.Header{
			Name:     "./file.txt",
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
			Mode:     0644,
		})
		require.NoError(t, err)
		_, err = tw.Write(content)
		require.NoError(t, err)

		err = tw.Close()
		require.NoError(t, err)

		s.mockAPI.MockGetArtifactDownloadRequest = func(taskId, artifactKey string) (api.ArtifactDownloadRequestResult, error) {
			return api.ArtifactDownloadRequestResult{
				URL:      "https://example.com/artifact",
				Filename: "task-444-dotslash.tar",
				Kind:     "directory",
				Key:      "dotslash",
			}, nil
		}

		s.mockAPI.MockDownloadArtifact = func(request api.ArtifactDownloadRequestResult) ([]byte, error) {
			return buf.Bytes(), nil
		}

		_, err = s.service.DownloadArtifact(cli.DownloadArtifactConfig{
			TaskID:      "task-444",
			ArtifactKey: "dotslash",
			OutputDir:   s.tmp,
			AutoExtract: true,
		})

		require.NoError(t, err)
		extractDir := filepath.Join(s.tmp, "task-444-dotslash")
		require.FileExists(t, filepath.Join(extractDir, "file.txt"))

		actualContents, err := os.ReadFile(filepath.Join(extractDir, "file.txt"))
		require.NoError(t, err)
		require.Equal(t, content, actualContents)
	})

	t.Run("when filename contains path traversal attempt - sanitizes directory name", func(t *testing.T) {
		s := setupTest(t)

		tarBytes := createTestTar(t, map[string][]byte{
			"safe.txt": []byte("safe content"),
		})

		s.mockAPI.MockGetArtifactDownloadRequest = func(taskId, artifactKey string) (api.ArtifactDownloadRequestResult, error) {
			return api.ArtifactDownloadRequestResult{
				URL:      "https://example.com/artifact",
				Filename: "../../etc/evil.tar", // Path traversal attempt
				Kind:     "file",
				Key:      "evil",
			}, nil
		}

		s.mockAPI.MockDownloadArtifact = func(request api.ArtifactDownloadRequestResult) ([]byte, error) {
			return tarBytes, nil
		}

		_, err := s.service.DownloadArtifact(cli.DownloadArtifactConfig{
			TaskID:      "task-999",
			ArtifactKey: "evil",
			OutputDir:   s.tmp,
		})

		require.NoError(t, err)
		// Should extract to safe sanitized directory name "evil" instead of "../../etc/evil"
		extractDir := filepath.Join(s.tmp, "evil")
		require.FileExists(t, filepath.Join(extractDir, "safe.txt"))

		actualContents, err := os.ReadFile(filepath.Join(extractDir, "safe.txt"))
		require.NoError(t, err)
		require.Equal(t, []byte("safe content"), actualContents)

		// Verify file was NOT created outside the temp directory
		evilPath := filepath.Join(s.tmp, "..", "..", "etc", "evil", "safe.txt")
		require.NoFileExists(t, evilPath)
	})
}

func TestService_ListArtifacts(t *testing.T) {
	t.Run("when validation fails - missing task ID", func(t *testing.T) {
		s := setupTest(t)

		_, err := s.service.ListArtifacts(cli.ListArtifactsConfig{
			TaskID: "",
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "validation failed")
		require.Contains(t, err.Error(), "task ID must be provided")
	})

	t.Run("when task is not found", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockGetAllArtifactDownloadRequests = func(taskId string) ([]api.ArtifactDownloadRequestResult, error) {
			return nil, api.ErrNotFound
		}

		_, err := s.service.ListArtifacts(cli.ListArtifactsConfig{
			TaskID: "task-999",
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "Artifacts for task task-999 not found")
	})

	t.Run("when API fails with other error", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockGetAllArtifactDownloadRequests = func(taskId string) ([]api.ArtifactDownloadRequestResult, error) {
			return nil, errors.New("network error")
		}

		_, err := s.service.ListArtifacts(cli.ListArtifactsConfig{
			TaskID: "task-123",
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "unable to fetch artifacts")
	})

	t.Run("lists no artifacts with text output", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockGetAllArtifactDownloadRequests = func(taskId string) ([]api.ArtifactDownloadRequestResult, error) {
			return []api.ArtifactDownloadRequestResult{}, nil
		}

		result, err := s.service.ListArtifacts(cli.ListArtifactsConfig{
			TaskID: "task-123",
		})

		require.NoError(t, err)
		require.Empty(t, result.Artifacts)
		require.Contains(t, s.mockStdout.String(), "No artifacts found for task task-123")
	})

	t.Run("lists multiple artifacts with text output", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockGetAllArtifactDownloadRequests = func(taskId string) ([]api.ArtifactDownloadRequestResult, error) {
			require.Equal(t, "task-123", taskId)
			return []api.ArtifactDownloadRequestResult{
				{URL: "https://example.com/a", Filename: "task-123~artifact-a.tar", SizeInBytes: 1024, Kind: "file", Key: "artifact-a"},
				{URL: "https://example.com/b", Filename: "task-123~artifact-b.tar", SizeInBytes: 2097152, Kind: "directory", Key: "artifact-b"},
			}, nil
		}

		result, err := s.service.ListArtifacts(cli.ListArtifactsConfig{
			TaskID: "task-123",
		})

		require.NoError(t, err)
		require.Len(t, result.Artifacts, 2)
		require.Equal(t, "artifact-a", result.Artifacts[0].Key)
		require.Equal(t, "artifact-b", result.Artifacts[1].Key)

		output := s.mockStdout.String()
		require.Contains(t, output, "KEY")
		require.Contains(t, output, "KIND")
		require.Contains(t, output, "SIZE")
		require.Contains(t, output, "artifact-a")
		require.Contains(t, output, "file")
		require.Contains(t, output, "1.0 KB")
		require.Contains(t, output, "artifact-b")
		require.Contains(t, output, "directory")
		require.Contains(t, output, "2.0 MB")
	})

	t.Run("lists artifacts with JSON output", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockGetAllArtifactDownloadRequests = func(taskId string) ([]api.ArtifactDownloadRequestResult, error) {
			return []api.ArtifactDownloadRequestResult{
				{URL: "https://example.com/a", Filename: "task-123~artifact-a.tar", SizeInBytes: 512, Kind: "file", Key: "artifact-a"},
			}, nil
		}

		result, err := s.service.ListArtifacts(cli.ListArtifactsConfig{
			TaskID: "task-123",
			Json:   true,
		})

		require.NoError(t, err)
		require.Len(t, result.Artifacts, 1)

		output := s.mockStdout.String()
		require.Contains(t, output, `"Artifacts"`)
		require.Contains(t, output, `"Key":"artifact-a"`)
		require.Contains(t, output, `"Kind":"file"`)
		require.Contains(t, output, `"SizeInBytes":512`)
		require.NotContains(t, output, "Artifacts for task")
	})

	t.Run("lists empty artifacts with JSON output", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockGetAllArtifactDownloadRequests = func(taskId string) ([]api.ArtifactDownloadRequestResult, error) {
			return []api.ArtifactDownloadRequestResult{}, nil
		}

		_, err := s.service.ListArtifacts(cli.ListArtifactsConfig{
			TaskID: "task-123",
			Json:   true,
		})

		require.NoError(t, err)
		output := s.mockStdout.String()
		require.Contains(t, output, `"Artifacts"`)
		require.NotContains(t, output, "No artifacts found")
	})
}

func TestService_DownloadAllArtifacts(t *testing.T) {
	t.Run("when no artifacts are found", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockGetAllArtifactDownloadRequests = func(taskId string) ([]api.ArtifactDownloadRequestResult, error) {
			require.Equal(t, "task-123", taskId)
			return []api.ArtifactDownloadRequestResult{}, nil
		}

		result, err := s.service.DownloadAllArtifacts(cli.DownloadAllArtifactsConfig{
			TaskID:    "task-123",
			OutputDir: s.tmp,
		})

		require.NoError(t, err)
		require.Empty(t, result.OutputFiles)
		require.Contains(t, s.mockStdout.String(), "No artifacts found for task task-123")
	})

	t.Run("when task is not found", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockGetAllArtifactDownloadRequests = func(taskId string) ([]api.ArtifactDownloadRequestResult, error) {
			return nil, api.ErrNotFound
		}

		_, err := s.service.DownloadAllArtifacts(cli.DownloadAllArtifactsConfig{
			TaskID:    "task-999",
			OutputDir: s.tmp,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "Artifacts for task task-999 not found")
	})

	t.Run("when GetAllArtifactDownloadRequests fails with other error", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockGetAllArtifactDownloadRequests = func(taskId string) ([]api.ArtifactDownloadRequestResult, error) {
			return nil, errors.New("network error")
		}

		_, err := s.service.DownloadAllArtifacts(cli.DownloadAllArtifactsConfig{
			TaskID:    "task-123",
			OutputDir: s.tmp,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "unable to fetch artifact download requests")
		require.Contains(t, err.Error(), "network error")
	})

	t.Run("when validation fails - missing task ID", func(t *testing.T) {
		s := setupTest(t)

		_, err := s.service.DownloadAllArtifacts(cli.DownloadAllArtifactsConfig{
			TaskID:    "",
			OutputDir: s.tmp,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "validation failed")
		require.Contains(t, err.Error(), "task ID must be provided")
	})

	t.Run("downloads multiple file artifacts", func(t *testing.T) {
		s := setupTest(t)

		tar1 := createTestTar(t, map[string][]byte{
			"file-a.txt": []byte("content a"),
		})
		tar2 := createTestTar(t, map[string][]byte{
			"file-b.txt": []byte("content b"),
		})

		s.mockAPI.MockGetAllArtifactDownloadRequests = func(taskId string) ([]api.ArtifactDownloadRequestResult, error) {
			return []api.ArtifactDownloadRequestResult{
				{URL: "https://example.com/a", Filename: "task-123~artifact-a.tar", Kind: "file", Key: "artifact-a"},
				{URL: "https://example.com/b", Filename: "task-123~artifact-b.tar", Kind: "file", Key: "artifact-b"},
			}, nil
		}

		s.mockAPI.MockDownloadArtifact = func(request api.ArtifactDownloadRequestResult) ([]byte, error) {
			if request.URL == "https://example.com/a" {
				return tar1, nil
			}
			return tar2, nil
		}

		result, err := s.service.DownloadAllArtifacts(cli.DownloadAllArtifactsConfig{
			TaskID:    "task-123",
			OutputDir: s.tmp,
		})

		require.NoError(t, err)
		require.Len(t, result.OutputFiles, 2)

		contentA, err := os.ReadFile(filepath.Join(s.tmp, "task-123~artifact-a", "file-a.txt"))
		require.NoError(t, err)
		require.Equal(t, []byte("content a"), contentA)

		contentB, err := os.ReadFile(filepath.Join(s.tmp, "task-123~artifact-b", "file-b.txt"))
		require.NoError(t, err)
		require.Equal(t, []byte("content b"), contentB)

		output := s.mockStdout.String()
		require.Contains(t, output, "Downloaded 2 artifact(s)")
	})

	t.Run("downloads directory artifact without auto-extract saves tar", func(t *testing.T) {
		s := setupTest(t)

		tarBytes := createTestTar(t, map[string][]byte{
			"file1.txt": []byte("content 1"),
		})

		s.mockAPI.MockGetAllArtifactDownloadRequests = func(taskId string) ([]api.ArtifactDownloadRequestResult, error) {
			return []api.ArtifactDownloadRequestResult{
				{URL: "https://example.com/a", Filename: "task-123~my-dir.tar", Kind: "directory", Key: "my-dir"},
			}, nil
		}

		s.mockAPI.MockDownloadArtifact = func(request api.ArtifactDownloadRequestResult) ([]byte, error) {
			return tarBytes, nil
		}

		result, err := s.service.DownloadAllArtifacts(cli.DownloadAllArtifactsConfig{
			TaskID:      "task-123",
			OutputDir:   s.tmp,
			AutoExtract: false,
		})

		require.NoError(t, err)
		require.Len(t, result.OutputFiles, 1)
		expectedPath := filepath.Join(s.tmp, "task-123~my-dir.tar")
		require.FileExists(t, expectedPath)

		actualContents, err := os.ReadFile(expectedPath)
		require.NoError(t, err)
		require.Equal(t, tarBytes, actualContents)
	})

	t.Run("downloads directory artifact with auto-extract", func(t *testing.T) {
		s := setupTest(t)

		tarBytes := createTestTar(t, map[string][]byte{
			"file1.txt": []byte("content 1"),
			"file2.txt": []byte("content 2"),
		})

		s.mockAPI.MockGetAllArtifactDownloadRequests = func(taskId string) ([]api.ArtifactDownloadRequestResult, error) {
			return []api.ArtifactDownloadRequestResult{
				{URL: "https://example.com/a", Filename: "task-123~my-dir.tar", Kind: "directory", Key: "my-dir"},
			}, nil
		}

		s.mockAPI.MockDownloadArtifact = func(request api.ArtifactDownloadRequestResult) ([]byte, error) {
			return tarBytes, nil
		}

		result, err := s.service.DownloadAllArtifacts(cli.DownloadAllArtifactsConfig{
			TaskID:      "task-123",
			OutputDir:   s.tmp,
			AutoExtract: true,
		})

		require.NoError(t, err)
		require.Len(t, result.OutputFiles, 2)

		extractDir := filepath.Join(s.tmp, "task-123~my-dir")
		require.FileExists(t, filepath.Join(extractDir, "file1.txt"))
		require.FileExists(t, filepath.Join(extractDir, "file2.txt"))

		output := s.mockStdout.String()
		require.Contains(t, output, "Extracted 2 file(s)")
	})

	t.Run("when one download fails", func(t *testing.T) {
		s := setupTest(t)

		tar1 := createTestTar(t, map[string][]byte{
			"file-a.txt": []byte("content a"),
		})

		s.mockAPI.MockGetAllArtifactDownloadRequests = func(taskId string) ([]api.ArtifactDownloadRequestResult, error) {
			return []api.ArtifactDownloadRequestResult{
				{URL: "https://example.com/a", Filename: "task-123~artifact-a.tar", Kind: "file", Key: "artifact-a"},
				{URL: "https://example.com/b", Filename: "task-123~artifact-b.tar", Kind: "file", Key: "artifact-b"},
			}, nil
		}

		s.mockAPI.MockDownloadArtifact = func(request api.ArtifactDownloadRequestResult) ([]byte, error) {
			if request.URL == "https://example.com/a" {
				return tar1, nil
			}
			return nil, errors.New("download failed")
		}

		_, err := s.service.DownloadAllArtifacts(cli.DownloadAllArtifactsConfig{
			TaskID:    "task-123",
			OutputDir: s.tmp,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "unable to download artifact artifact-b")
	})

	t.Run("with JSON output", func(t *testing.T) {
		s := setupTest(t)

		tar1 := createTestTar(t, map[string][]byte{
			"file-a.txt": []byte("content a"),
		})

		s.mockAPI.MockGetAllArtifactDownloadRequests = func(taskId string) ([]api.ArtifactDownloadRequestResult, error) {
			return []api.ArtifactDownloadRequestResult{
				{URL: "https://example.com/a", Filename: "task-123~artifact-a.tar", Kind: "file", Key: "artifact-a"},
			}, nil
		}

		s.mockAPI.MockDownloadArtifact = func(request api.ArtifactDownloadRequestResult) ([]byte, error) {
			return tar1, nil
		}

		_, err := s.service.DownloadAllArtifacts(cli.DownloadAllArtifactsConfig{
			TaskID:    "task-123",
			OutputDir: s.tmp,
			Json:      true,
		})

		require.NoError(t, err)
		output := s.mockStdout.String()
		require.Contains(t, output, `"OutputFiles"`)
		require.Contains(t, output, "file-a.txt")
		require.NotContains(t, output, "Downloaded")
	})

	t.Run("with explicit output dir extracts directly into it", func(t *testing.T) {
		s := setupTest(t)

		tar1 := createTestTar(t, map[string][]byte{
			"file-a.txt": []byte("content a"),
		})

		s.mockAPI.MockGetAllArtifactDownloadRequests = func(taskId string) ([]api.ArtifactDownloadRequestResult, error) {
			return []api.ArtifactDownloadRequestResult{
				{URL: "https://example.com/a", Filename: "task-123~artifact-a.tar", Kind: "file", Key: "artifact-a"},
			}, nil
		}

		s.mockAPI.MockDownloadArtifact = func(request api.ArtifactDownloadRequestResult) ([]byte, error) {
			return tar1, nil
		}

		customDir := filepath.Join(s.tmp, "custom-output")
		require.NoError(t, os.MkdirAll(customDir, 0755))

		result, err := s.service.DownloadAllArtifacts(cli.DownloadAllArtifactsConfig{
			TaskID:                 "task-123",
			OutputDir:              customDir,
			OutputDirExplicitlySet: true,
		})

		require.NoError(t, err)
		require.Len(t, result.OutputFiles, 1)
		require.FileExists(t, filepath.Join(customDir, "file-a.txt"))
	})
}

func createTestTar(t *testing.T, files map[string][]byte) []byte {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	for name, content := range files {
		// Create directory entries if needed
		if dir := filepath.Dir(name); dir != "." {
			err := tw.WriteHeader(&tar.Header{
				Name:     dir + "/",
				Typeflag: tar.TypeDir,
				Mode:     0755,
			})
			require.NoError(t, err)
		}

		// Create file entry
		err := tw.WriteHeader(&tar.Header{
			Name:     name,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
			Mode:     0644,
		})
		require.NoError(t, err)

		_, err = tw.Write(content)
		require.NoError(t, err)
	}

	err := tw.Close()
	require.NoError(t, err)

	return buf.Bytes()
}
