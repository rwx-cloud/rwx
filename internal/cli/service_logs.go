package cli

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/skratchdot/open-golang/open"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type DownloadLogsConfig struct {
	TaskID                 string
	RunID                  string
	TaskKey                string
	OutputDir              string
	OutputFile             string
	OutputDirExplicitlySet bool
	Json                   bool
	Zip                    bool
	Open                   bool
}

func (c DownloadLogsConfig) Validate() error {
	if c.TaskKey != "" {
		if c.RunID == "" {
			return errors.New("run ID must be provided when using task key")
		}
	} else if c.TaskID == "" {
		return errors.New("task ID must be provided")
	}
	if c.OutputDir != "" && c.OutputFile != "" {
		return errors.New("output-dir and output-file cannot be used together")
	}
	if !c.Zip && c.OutputFile != "" {
		return errors.New("output-file can only be used with --zip")
	}
	if c.OutputDir == "" && c.OutputFile == "" {
		return errors.New("output directory or output file must be provided")
	}
	return nil
}

type DownloadLogsResult struct {
	OutputFiles []string
}

func (s Service) DownloadLogs(cfg DownloadLogsConfig) (_ *DownloadLogsResult, dlErr error) {
	start := time.Now()
	defer func() {
		s.recordTelemetry("logs.download", map[string]any{
			"duration_ms": time.Since(start).Milliseconds(),
			"zip":         cfg.Zip,
		})
	}()

	err := cfg.Validate()
	if err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	var logDownloadRequest api.LogDownloadRequestResult
	if cfg.TaskKey != "" {
		logDownloadRequest, err = s.APIClient.GetLogDownloadRequestByTaskKey(cfg.RunID, cfg.TaskKey)
	} else {
		logDownloadRequest, err = s.APIClient.GetLogDownloadRequest(cfg.TaskID)
	}
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			if cfg.TaskKey != "" {
				return nil, errors.WrapSentinel(errors.New(fmt.Sprintf("Task with key '%s' not found", cfg.TaskKey)), api.ErrNotFound)
			}
			return nil, errors.WrapSentinel(errors.New(fmt.Sprintf("Task %s not found", cfg.TaskID)), api.ErrNotFound)
		}
		return nil, errors.Wrap(err, "unable to fetch log archive request")
	}

	stopSpinner := Spin(
		"Downloading logs...",
		s.StderrIsTTY,
		s.Stderr,
	)

	logBytes, err := s.APIClient.DownloadLogs(logDownloadRequest)
	stopSpinner()
	if err != nil {
		return nil, errors.Wrap(err, "unable to download logs")
	}

	var outputFiles []string

	if cfg.Zip {
		zipPath := cfg.OutputFile
		if zipPath == "" {
			zipPath = filepath.Join(cfg.OutputDir, logDownloadRequest.Filename)
		}

		if err := os.MkdirAll(filepath.Dir(zipPath), 0755); err != nil {
			return nil, errors.Wrapf(err, "unable to create output directory %s", filepath.Dir(zipPath))
		}

		if err := os.WriteFile(zipPath, logBytes, 0644); err != nil {
			return nil, errors.Wrapf(err, "unable to write zip file to %s", zipPath)
		}

		outputFiles = []string{zipPath}

		if !cfg.Json {
			fmt.Fprintf(s.Stdout, "Logs downloaded to %s\n", zipPath)
		}
	} else {
		extractDir := cfg.OutputDir
		if !cfg.OutputDirExplicitlySet {
			extractDir = filepath.Join(cfg.OutputDir, logDownloadRequest.RunID)
		}
		if err := os.MkdirAll(extractDir, 0755); err != nil {
			return nil, errors.Wrapf(err, "unable to create extraction directory %s", extractDir)
		}

		// Write zip bytes to a temp file so extractZip can open it by path.
		tmpFile, err := os.CreateTemp("", "rwx-logs-*.zip")
		if err != nil {
			return nil, errors.Wrap(err, "unable to create temporary file for zip")
		}
		tmpPath := tmpFile.Name()
		defer os.Remove(tmpPath)

		if _, err := tmpFile.Write(logBytes); err != nil {
			tmpFile.Close()
			return nil, errors.Wrap(err, "unable to write temporary zip file")
		}
		tmpFile.Close()

		extractedFiles, err := extractZip(tmpPath, extractDir)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to extract zip archive")
		}
		outputFiles = extractedFiles

		if !cfg.Json {
			fmt.Fprintf(s.Stdout, "Logs downloaded to %s/\n", extractDir)
			for _, file := range outputFiles {
				fmt.Fprintf(s.Stdout, "  %s\n", file)
			}
		}
	}

	if cfg.Open {
		for _, file := range outputFiles {
			if err := open.Run(file); err != nil {
				if !cfg.Json {
					fmt.Fprintf(s.Stderr, "Failed to open %s: %v\n", file, err)
				}
			}
		}
	}

	result := &DownloadLogsResult{OutputFiles: outputFiles}

	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	}

	return result, nil
}

func extractZip(zipPath, destDir string) ([]string, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, errors.Wrap(err, "unable to open zip file")
	}
	defer reader.Close()

	var extractedFiles []string

	for _, file := range reader.File {
		filePath := filepath.Join(destDir, file.Name)
		if !strings.HasPrefix(filePath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return nil, fmt.Errorf("invalid file path in zip: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(filePath, 0755); err != nil {
				return nil, errors.Wrapf(err, "unable to create directory %s", filePath)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return nil, errors.Wrapf(err, "unable to create directory for %s", filePath)
		}

		rc, err := file.Open()
		if err != nil {
			return nil, errors.Wrapf(err, "unable to open file %s in zip", file.Name)
		}

		outFile, err := os.Create(filePath)
		if err != nil {
			rc.Close()
			return nil, errors.Wrapf(err, "unable to create file %s", filePath)
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return nil, errors.Wrapf(err, "unable to extract file %s", filePath)
		}

		if err := os.Chmod(filePath, file.FileInfo().Mode()); err != nil {
			return nil, errors.Wrapf(err, "unable to set permissions for %s", filePath)
		}

		extractedFiles = append(extractedFiles, filePath)
	}

	return extractedFiles, nil
}
