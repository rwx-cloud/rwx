package cli_test

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestService_DownloadLogs(t *testing.T) {
	t.Run("when the task is not found", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockGetLogDownloadRequest = func(taskId string) (api.LogDownloadRequestResult, error) {
			require.Equal(t, "task-123", taskId)
			return api.LogDownloadRequestResult{}, api.ErrNotFound
		}

		_, err := s.service.DownloadLogs(cli.DownloadLogsConfig{
			TaskID:    "task-123",
			OutputDir: s.tmp,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "Task task-123 not found")
	})

	t.Run("when GetLogDownloadRequest fails with other error", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockGetLogDownloadRequest = func(taskId string) (api.LogDownloadRequestResult, error) {
			return api.LogDownloadRequestResult{}, errors.New("network error")
		}

		_, err := s.service.DownloadLogs(cli.DownloadLogsConfig{
			TaskID:    "task-123",
			OutputDir: s.tmp,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "unable to fetch log archive request")
		require.Contains(t, err.Error(), "network error")
	})

	t.Run("when DownloadLogs fails", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockGetLogDownloadRequest = func(taskId string) (api.LogDownloadRequestResult, error) {
			return api.LogDownloadRequestResult{
				URL:      "https://example.com/logs",
				Token:    "jwt-token",
				Filename: "task-123-logs.zip",
				RunID:    "run-abc",
			}, nil
		}

		s.mockAPI.MockDownloadLogs = func(request api.LogDownloadRequestResult) ([]byte, error) {
			return nil, errors.New("download failed")
		}

		_, err := s.service.DownloadLogs(cli.DownloadLogsConfig{
			TaskID:    "task-123",
			OutputDir: s.tmp,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "unable to download logs")
		require.Contains(t, err.Error(), "download failed")
		require.Contains(t, s.mockStderr.String(), "Downloading logs...")
	})

	t.Run("when output directory does not exist, it is created", func(t *testing.T) {
		s := setupTest(t)

		zipBytes := createTestZip(t, map[string][]byte{
			"task.log": []byte("log content"),
		})

		s.mockAPI.MockGetLogDownloadRequest = func(taskId string) (api.LogDownloadRequestResult, error) {
			return api.LogDownloadRequestResult{
				URL:      "https://example.com/logs",
				Token:    "jwt-token",
				Filename: "task-123-logs.zip",
				RunID:    "run-abc123",
			}, nil
		}

		s.mockAPI.MockDownloadLogs = func(request api.LogDownloadRequestResult) ([]byte, error) {
			return zipBytes, nil
		}

		nestedDir := filepath.Join(s.tmp, "nonexistent", "subdir")
		_, err := s.service.DownloadLogs(cli.DownloadLogsConfig{
			TaskID:    "task-123",
			OutputDir: nestedDir,
		})

		require.NoError(t, err)
		extractDir := filepath.Join(nestedDir, "run-abc123")
		require.FileExists(t, filepath.Join(extractDir, "task.log"))
		require.Contains(t, s.mockStderr.String(), "Downloading logs...")
	})

	t.Run("when download succeeds, extracts to run directory by default", func(t *testing.T) {
		s := setupTest(t)

		zipBytes := createTestZip(t, map[string][]byte{
			"ci.rspec.rspec-0.log": []byte("rspec log content"),
		})

		s.mockAPI.MockGetLogDownloadRequest = func(taskId string) (api.LogDownloadRequestResult, error) {
			require.Equal(t, "task-123", taskId)
			return api.LogDownloadRequestResult{
				URL:      "https://example.com/logs",
				Token:    "jwt-token",
				Filename: "task-123-logs.zip",
				RunID:    "d3adb33f",
			}, nil
		}

		s.mockAPI.MockDownloadLogs = func(request api.LogDownloadRequestResult) ([]byte, error) {
			require.Equal(t, "https://example.com/logs", request.URL)
			require.Equal(t, "jwt-token", request.Token)
			require.Equal(t, "task-123-logs.zip", request.Filename)
			return zipBytes, nil
		}

		result, err := s.service.DownloadLogs(cli.DownloadLogsConfig{
			TaskID:    "task-123",
			OutputDir: s.tmp,
		})

		require.NoError(t, err)

		extractDir := filepath.Join(s.tmp, "d3adb33f")
		require.FileExists(t, filepath.Join(extractDir, "ci.rspec.rspec-0.log"))

		output := s.mockStdout.String()
		require.Contains(t, output, "Logs downloaded to")
		require.Contains(t, output, "d3adb33f/")
		require.Contains(t, output, "ci.rspec.rspec-0.log")
		require.Contains(t, s.mockStderr.String(), "Downloading logs...")

		require.Len(t, result.OutputFiles, 1)
		require.Contains(t, result.OutputFiles[0], "ci.rspec.rspec-0.log")
	})

	t.Run("when explicit output directory is set, extracts directly into it", func(t *testing.T) {
		s := setupTest(t)

		zipBytes := createTestZip(t, map[string][]byte{
			"test.log": []byte("log content"),
		})

		s.mockAPI.MockGetLogDownloadRequest = func(taskId string) (api.LogDownloadRequestResult, error) {
			return api.LogDownloadRequestResult{
				URL:      "https://example.com/logs",
				Token:    "jwt-token",
				Filename: "task-123-logs.zip",
				RunID:    "run-explicit",
			}, nil
		}

		s.mockAPI.MockDownloadLogs = func(request api.LogDownloadRequestResult) ([]byte, error) {
			return zipBytes, nil
		}

		outputDir := filepath.Join(s.tmp, "requested-output")
		result, err := s.service.DownloadLogs(cli.DownloadLogsConfig{
			TaskID:                 "task-123",
			OutputDir:              outputDir,
			OutputDirExplicitlySet: true,
		})

		require.NoError(t, err)
		expectedPath := filepath.Join(outputDir, "test.log")
		require.FileExists(t, expectedPath)
		require.NoFileExists(t, filepath.Join(outputDir, "run-explicit", "test.log"))
		require.Equal(t, []string{expectedPath}, result.OutputFiles)

		output := s.mockStdout.String()
		require.Contains(t, output, "Logs downloaded to")
		require.Contains(t, output, outputDir+string(os.PathSeparator))
	})

	t.Run("when download succeeds with multiple log files", func(t *testing.T) {
		s := setupTest(t)

		zipBytes := createTestZip(t, map[string][]byte{
			"ci.rspec.rspec-0.log":              []byte("rspec log content"),
			"ci.rspec.rspec-0.bg.databases.log": []byte("bg log content"),
		})

		s.mockAPI.MockGetLogDownloadRequest = func(taskId string) (api.LogDownloadRequestResult, error) {
			return api.LogDownloadRequestResult{
				URL:      "https://example.com/logs",
				Token:    "jwt-token",
				Filename: "task-456-logs.zip",
				RunID:    "run-multi",
			}, nil
		}

		s.mockAPI.MockDownloadLogs = func(request api.LogDownloadRequestResult) ([]byte, error) {
			return zipBytes, nil
		}

		result, err := s.service.DownloadLogs(cli.DownloadLogsConfig{
			TaskID:    "task-456",
			OutputDir: s.tmp,
		})

		require.NoError(t, err)

		extractDir := filepath.Join(s.tmp, "run-multi")
		require.FileExists(t, filepath.Join(extractDir, "ci.rspec.rspec-0.log"))
		require.FileExists(t, filepath.Join(extractDir, "ci.rspec.rspec-0.bg.databases.log"))

		output := s.mockStdout.String()
		require.Contains(t, output, "Logs downloaded to")
		require.Contains(t, output, "run-multi/")
		require.Contains(t, output, "ci.rspec.rspec-0.log")
		require.Contains(t, output, "ci.rspec.rspec-0.bg.databases.log")

		require.Len(t, result.OutputFiles, 2)
	})

	t.Run("when re-download overwrites existing extracted files", func(t *testing.T) {
		s := setupTest(t)

		runID := "run-overwrite"
		extractDir := filepath.Join(s.tmp, runID)
		require.NoError(t, os.MkdirAll(extractDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(extractDir, "task.log"), []byte("old content"), 0644))

		zipBytes := createTestZip(t, map[string][]byte{
			"task.log": []byte("new content"),
		})

		s.mockAPI.MockGetLogDownloadRequest = func(taskId string) (api.LogDownloadRequestResult, error) {
			return api.LogDownloadRequestResult{
				URL:      "https://example.com/logs",
				Token:    "jwt-token",
				Filename: "task-logs.zip",
				RunID:    runID,
			}, nil
		}

		s.mockAPI.MockDownloadLogs = func(request api.LogDownloadRequestResult) ([]byte, error) {
			return zipBytes, nil
		}

		_, err := s.service.DownloadLogs(cli.DownloadLogsConfig{
			TaskID:    "task-123",
			OutputDir: s.tmp,
		})

		require.NoError(t, err)

		contents, err := os.ReadFile(filepath.Join(extractDir, "task.log"))
		require.NoError(t, err)
		require.Equal(t, []byte("new content"), contents)
	})

	t.Run("when validation fails - missing task ID", func(t *testing.T) {
		s := setupTest(t)

		_, err := s.service.DownloadLogs(cli.DownloadLogsConfig{
			TaskID:    "",
			OutputDir: s.tmp,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "validation failed")
		require.Contains(t, err.Error(), "task ID must be provided")
	})

	t.Run("when validation fails - missing output destination", func(t *testing.T) {
		s := setupTest(t)

		_, err := s.service.DownloadLogs(cli.DownloadLogsConfig{
			TaskID: "task-123",
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "validation failed")
		require.Contains(t, err.Error(), "output directory or output file must be provided")
	})

	t.Run("when validation fails - output file set while extracting", func(t *testing.T) {
		s := setupTest(t)

		_, err := s.service.DownloadLogs(cli.DownloadLogsConfig{
			TaskID:     "task-123",
			OutputFile: filepath.Join(s.tmp, "task.log"),
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "validation failed")
		require.Contains(t, err.Error(), "output-file can only be used with --zip")
	})

	t.Run("when validation fails - both output dir and output file set", func(t *testing.T) {
		s := setupTest(t)

		_, err := s.service.DownloadLogs(cli.DownloadLogsConfig{
			TaskID:     "task-123",
			OutputDir:  s.tmp,
			OutputFile: filepath.Join(s.tmp, "task.zip"),
			Zip:        true,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "validation failed")
		require.Contains(t, err.Error(), "output-dir and output-file cannot be used together")
	})

	t.Run("when --zip flag saves raw zip without extraction", func(t *testing.T) {
		s := setupTest(t)

		zipBytes := createTestZip(t, map[string][]byte{
			"task.log": []byte("log content"),
		})

		s.mockAPI.MockGetLogDownloadRequest = func(taskId string) (api.LogDownloadRequestResult, error) {
			return api.LogDownloadRequestResult{
				URL:      "https://example.com/logs",
				Token:    "jwt-token",
				Filename: "task-123-logs.zip",
				RunID:    "d3adb33f",
			}, nil
		}

		s.mockAPI.MockDownloadLogs = func(request api.LogDownloadRequestResult) ([]byte, error) {
			return zipBytes, nil
		}

		result, err := s.service.DownloadLogs(cli.DownloadLogsConfig{
			TaskID:    "task-123",
			OutputDir: s.tmp,
			Zip:       true,
		})

		require.NoError(t, err)

		zipPath := filepath.Join(s.tmp, "task-123-logs.zip")
		require.FileExists(t, zipPath)

		// Should not have created the run directory
		require.NoDirExists(t, filepath.Join(s.tmp, "d3adb33f"))

		actualBytes, err := os.ReadFile(zipPath)
		require.NoError(t, err)
		require.Equal(t, zipBytes, actualBytes)

		output := s.mockStdout.String()
		require.Contains(t, output, "Logs downloaded to")
		require.Contains(t, output, "task-123-logs.zip")

		require.Equal(t, []string{zipPath}, result.OutputFiles)
	})

	t.Run("when --zip flag with output-file", func(t *testing.T) {
		s := setupTest(t)

		zipBytes := createTestZip(t, map[string][]byte{
			"task.log": []byte("log content"),
		})
		customPath := filepath.Join(s.tmp, "custom", "archive.zip")

		s.mockAPI.MockGetLogDownloadRequest = func(taskId string) (api.LogDownloadRequestResult, error) {
			return api.LogDownloadRequestResult{
				URL:      "https://example.com/logs",
				Token:    "jwt-token",
				Filename: "task-123-logs.zip",
				RunID:    "d3adb33f",
			}, nil
		}

		s.mockAPI.MockDownloadLogs = func(request api.LogDownloadRequestResult) ([]byte, error) {
			return zipBytes, nil
		}

		_, err := s.service.DownloadLogs(cli.DownloadLogsConfig{
			TaskID:     "task-123",
			OutputFile: customPath,
			Zip:        true,
		})

		require.NoError(t, err)
		require.FileExists(t, customPath)
		actualBytes, err := os.ReadFile(customPath)
		require.NoError(t, err)
		require.Equal(t, zipBytes, actualBytes)
	})

	t.Run("when --zip flag with JSON output", func(t *testing.T) {
		s := setupTest(t)

		zipBytes := createTestZip(t, map[string][]byte{
			"task.log": []byte("log content"),
		})

		s.mockAPI.MockGetLogDownloadRequest = func(taskId string) (api.LogDownloadRequestResult, error) {
			return api.LogDownloadRequestResult{
				URL:      "https://example.com/logs",
				Token:    "jwt-token",
				Filename: "task-123-logs.zip",
				RunID:    "d3adb33f",
			}, nil
		}

		s.mockAPI.MockDownloadLogs = func(request api.LogDownloadRequestResult) ([]byte, error) {
			return zipBytes, nil
		}

		_, err := s.service.DownloadLogs(cli.DownloadLogsConfig{
			TaskID:    "task-123",
			OutputDir: s.tmp,
			Zip:       true,
			Json:      true,
		})

		require.NoError(t, err)
		output := s.mockStdout.String()
		require.Contains(t, output, `"OutputFiles"`)
		require.Contains(t, output, "task-123-logs.zip")
		require.NotContains(t, output, "Logs downloaded to")
	})

	t.Run("when default mode with JSON output", func(t *testing.T) {
		s := setupTest(t)

		zipBytes := createTestZip(t, map[string][]byte{
			"file1.log":        []byte("log content 1"),
			"file2.log":        []byte("log content 2"),
			"subdir/file3.log": []byte("log content 3"),
		})

		s.mockAPI.MockGetLogDownloadRequest = func(taskId string) (api.LogDownloadRequestResult, error) {
			return api.LogDownloadRequestResult{
				URL:      "https://example.com/logs",
				Token:    "jwt-token",
				Filename: "task-456-logs.zip",
				RunID:    "run-json",
			}, nil
		}

		s.mockAPI.MockDownloadLogs = func(request api.LogDownloadRequestResult) ([]byte, error) {
			return zipBytes, nil
		}

		_, err := s.service.DownloadLogs(cli.DownloadLogsConfig{
			TaskID:    "task-456",
			OutputDir: s.tmp,
			Json:      true,
		})

		require.NoError(t, err)

		extractDir := filepath.Join(s.tmp, "run-json")
		require.FileExists(t, filepath.Join(extractDir, "file1.log"))
		require.FileExists(t, filepath.Join(extractDir, "file2.log"))
		require.FileExists(t, filepath.Join(extractDir, "subdir", "file3.log"))

		output := s.mockStdout.String()
		require.Contains(t, output, `"OutputFiles"`)
		require.Contains(t, output, "run-json")
		require.NotContains(t, output, "Logs downloaded to")
	})

	t.Run("when --auto-extract flag is passed, it is a no-op (still extracts)", func(t *testing.T) {
		s := setupTest(t)

		zipBytes := createTestZip(t, map[string][]byte{
			"task.log": []byte("log content"),
		})

		s.mockAPI.MockGetLogDownloadRequest = func(taskId string) (api.LogDownloadRequestResult, error) {
			return api.LogDownloadRequestResult{
				URL:      "https://example.com/logs",
				Token:    "jwt-token",
				Filename: "task-123-logs.zip",
				RunID:    "run-noop",
			}, nil
		}

		s.mockAPI.MockDownloadLogs = func(request api.LogDownloadRequestResult) ([]byte, error) {
			return zipBytes, nil
		}

		// AutoExtract is gone from DownloadLogsConfig; --auto-extract is a no-op at the CLI layer.
		// The service always extracts by default (Zip=false).
		_, err := s.service.DownloadLogs(cli.DownloadLogsConfig{
			TaskID:    "task-123",
			OutputDir: s.tmp,
			Zip:       false,
		})

		require.NoError(t, err)
		require.FileExists(t, filepath.Join(s.tmp, "run-noop", "task.log"))
	})
}

func createTestZip(t *testing.T, files map[string][]byte) []byte {
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)

	for name, content := range files {
		if filepath.Dir(name) != "." {
			_, err := writer.Create(filepath.Dir(name) + "/")
			require.NoError(t, err)
		}

		fileWriter, err := writer.Create(name)
		require.NoError(t, err)

		_, err = fileWriter.Write(content)
		require.NoError(t, err)
	}

	err := writer.Close()
	require.NoError(t, err)

	return buf.Bytes()
}
