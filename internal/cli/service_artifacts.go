package cli

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/skratchdot/open-golang/open"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type DownloadArtifactConfig struct {
	TaskID              string
	RunID               string
	TaskKey             string
	ArtifactKey         string
	Output              string
	OutputExplicitlySet bool
	Json                bool
	AutoExtract         bool
	Open                bool
}

func (c DownloadArtifactConfig) Validate() error {
	if c.TaskKey != "" {
		if c.RunID == "" {
			return errors.New("run ID must be provided when using task key")
		}
	} else if c.TaskID == "" {
		return errors.New("task ID must be provided")
	}
	if c.ArtifactKey == "" {
		return errors.New("artifact key must be provided")
	}
	if c.Output == "" {
		return errors.New("output must be provided")
	}
	return nil
}

type DownloadArtifactResult struct {
	OutputFiles []string
}

func (s Service) DownloadArtifact(cfg DownloadArtifactConfig) (_ *DownloadArtifactResult, dlErr error) {
	start := time.Now()
	var totalBytes int64
	defer func() {
		s.recordTelemetry("artifacts.download", map[string]any{
			"count":        1,
			"total_bytes":  totalBytes,
			"duration_ms":  time.Since(start).Milliseconds(),
			"auto_extract": cfg.AutoExtract,
		})
	}()

	err := cfg.Validate()
	if err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	var artifactDownloadRequest api.ArtifactDownloadRequestResult
	if cfg.TaskKey != "" {
		artifactDownloadRequest, err = s.APIClient.GetArtifactDownloadRequestByTaskKey(cfg.RunID, cfg.TaskKey, cfg.ArtifactKey)
	} else {
		artifactDownloadRequest, err = s.APIClient.GetArtifactDownloadRequest(cfg.TaskID, cfg.ArtifactKey)
	}
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			if cfg.TaskKey != "" {
				return nil, errors.WrapSentinel(errors.New(fmt.Sprintf("Artifact %s for task key '%s' not found", cfg.ArtifactKey, cfg.TaskKey)), api.ErrNotFound)
			}
			return nil, errors.WrapSentinel(errors.New(fmt.Sprintf("Artifact %s for task %s not found", cfg.ArtifactKey, cfg.TaskID)), api.ErrNotFound)
		}
		return nil, errors.Wrap(err, "unable to fetch artifact download request")
	}

	totalBytes = artifactDownloadRequest.SizeInBytes

	stopSpinner := Spin(
		"Downloading artifact...",
		s.StderrIsTTY,
		s.Stderr,
	)

	artifactBytes, err := s.APIClient.DownloadArtifact(artifactDownloadRequest)
	stopSpinner()
	if err != nil {
		return nil, errors.Wrap(err, "unable to download artifact")
	}

	// For files, always extract the single file from the tar
	// For directories, extract if AutoExtract is true
	shouldExtract := artifactDownloadRequest.Kind == "file" || (artifactDownloadRequest.Kind == "directory" && cfg.AutoExtract)

	var outputFiles []string

	if shouldExtract {
		if artifactDownloadRequest.Kind == "file" && cfg.OutputExplicitlySet {
			outputFiles, err = extractSingleFileTar(artifactBytes, cfg.Output, artifactDownloadRequest.Key)
			if err != nil {
				return nil, err
			}
		} else {
			var extractDir string
			if cfg.OutputExplicitlySet {
				extractDir = cfg.Output
			} else {
				extractDir = filepath.Join(cfg.Output, artifactStem(artifactDownloadRequest.Filename))
			}

			if err := prepareDirectoryOutput(extractDir); err != nil {
				return nil, errors.Wrapf(err, "unable to create extraction directory %s", extractDir)
			}

			extractedFiles, err := extractTar(artifactBytes, extractDir)
			if err != nil {
				return nil, errors.Wrapf(err, "unable to extract tar archive")
			}

			outputFiles = extractedFiles

			if !cfg.Json && artifactDownloadRequest.Kind == "directory" {
				fmt.Fprintf(s.Stdout, "Extracted %d file(s) to %s\n", len(outputFiles), extractDir)
			}
		}
	} else {
		// Save the raw tar file
		var outputPath string
		if cfg.OutputExplicitlySet {
			outputPath = cfg.Output
		} else {
			outputPath = filepath.Join(cfg.Output, downloadFilename(artifactDownloadRequest.Filename))
		}

		if err := prepareFileOutput(outputPath); err != nil {
			return nil, err
		}

		if _, err := os.Stat(outputPath); err == nil {
			if !cfg.Json {
				fmt.Fprintf(s.Stdout, "Overwriting existing file at %s\n", outputPath)
			}
		}

		if err := os.WriteFile(outputPath, artifactBytes, 0644); err != nil {
			return nil, errors.Wrapf(err, "unable to write artifact file to %s", outputPath)
		}

		outputFiles = []string{outputPath}
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

	result := &DownloadArtifactResult{OutputFiles: outputFiles}

	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		if len(outputFiles) == 1 {
			fmt.Fprintf(s.Stdout, "Artifact downloaded to %s\n", outputFiles[0])
		} else {
			fmt.Fprintf(s.Stdout, "Artifact downloaded and extracted:\n")
			for _, file := range outputFiles {
				fmt.Fprintf(s.Stdout, "  %s\n", file)
			}
		}
	}

	return result, nil
}

type ListArtifactsConfig struct {
	TaskID  string
	RunID   string
	TaskKey string
	Json    bool
}

func (c ListArtifactsConfig) Validate() error {
	if c.TaskKey != "" {
		if c.RunID == "" {
			return errors.New("run ID must be provided when using task key")
		}
	} else if c.TaskID == "" {
		return errors.New("task ID must be provided")
	}
	return nil
}

type ArtifactInfo struct {
	Key         string
	Kind        string
	SizeInBytes int64
}

type ListArtifactsResult struct {
	Artifacts []ArtifactInfo
}

func (s Service) ListArtifacts(cfg ListArtifactsConfig) (*ListArtifactsResult, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	var artifactDownloadRequests []api.ArtifactDownloadRequestResult
	if cfg.TaskKey != "" {
		artifactDownloadRequests, err = s.APIClient.GetAllArtifactDownloadRequestsByTaskKey(cfg.RunID, cfg.TaskKey)
	} else {
		artifactDownloadRequests, err = s.APIClient.GetAllArtifactDownloadRequests(cfg.TaskID)
	}
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			if cfg.TaskKey != "" {
				return nil, errors.WrapSentinel(errors.New(fmt.Sprintf("Artifacts for task key '%s' not found", cfg.TaskKey)), api.ErrNotFound)
			}
			return nil, errors.WrapSentinel(errors.New(fmt.Sprintf("Artifacts for task %s not found", cfg.TaskID)), api.ErrNotFound)
		}
		return nil, errors.Wrap(err, "unable to fetch artifacts")
	}

	artifacts := make([]ArtifactInfo, len(artifactDownloadRequests))
	for i, req := range artifactDownloadRequests {
		artifacts[i] = ArtifactInfo{
			Key:         req.Key,
			Kind:        req.Kind,
			SizeInBytes: req.SizeInBytes,
		}
	}

	result := &ListArtifactsResult{Artifacts: artifacts}

	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		if len(artifacts) == 0 {
			fmt.Fprintf(s.Stdout, "No artifacts found for task %s\n", cfg.TaskID)
		} else {
			maxKeyLen := len("KEY")
			maxKindLen := len("KIND")
			maxSizeLen := len("SIZE")
			for _, a := range artifacts {
				if len(a.Key) > maxKeyLen {
					maxKeyLen = len(a.Key)
				}
				if len(a.Kind) > maxKindLen {
					maxKindLen = len(a.Kind)
				}
				if s := formatBytes(a.SizeInBytes); len(s) > maxSizeLen {
					maxSizeLen = len(s)
				}
			}
			fmtStr := fmt.Sprintf("%%-%ds  %%-%ds  %%s\n", maxKeyLen, maxKindLen)
			fmt.Fprintf(s.Stdout, fmtStr, "KEY", "KIND", "SIZE")
			for _, a := range artifacts {
				fmt.Fprintf(s.Stdout, fmtStr, a.Key, a.Kind, formatBytes(a.SizeInBytes))
			}
		}
	}

	return result, nil
}

func formatBytes(b int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func downloadAllArtifactsDirName(cfg DownloadAllArtifactsConfig) string {
	if cfg.TaskID != "" {
		return safePathComponent(cfg.TaskID+"-artifacts", "artifacts")
	}
	if cfg.RunID != "" && cfg.TaskKey != "" {
		return safePathComponent(cfg.RunID+"-"+cfg.TaskKey+"-artifacts", "artifacts")
	}
	return "artifacts"
}

type DownloadAllArtifactsConfig struct {
	TaskID              string
	RunID               string
	TaskKey             string
	Output              string
	OutputExplicitlySet bool
	Json                bool
	AutoExtract         bool
	Open                bool
}

func (c DownloadAllArtifactsConfig) Validate() error {
	if c.TaskKey != "" {
		if c.RunID == "" {
			return errors.New("run ID must be provided when using task key")
		}
	} else if c.TaskID == "" {
		return errors.New("task ID must be provided")
	}
	if c.Output == "" {
		return errors.New("output must be provided")
	}
	return nil
}

type DownloadAllArtifactsResult struct {
	OutputFiles []string
}

func (s Service) DownloadAllArtifacts(cfg DownloadAllArtifactsConfig) (_ *DownloadAllArtifactsResult, dlErr error) {
	start := time.Now()
	var artifactCount int
	var totalBytes int64
	defer func() {
		s.recordTelemetry("artifacts.download", map[string]any{
			"count":        artifactCount,
			"total_bytes":  totalBytes,
			"duration_ms":  time.Since(start).Milliseconds(),
			"auto_extract": cfg.AutoExtract,
		})
	}()

	err := cfg.Validate()
	if err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	var artifactDownloadRequests []api.ArtifactDownloadRequestResult
	if cfg.TaskKey != "" {
		artifactDownloadRequests, err = s.APIClient.GetAllArtifactDownloadRequestsByTaskKey(cfg.RunID, cfg.TaskKey)
	} else {
		artifactDownloadRequests, err = s.APIClient.GetAllArtifactDownloadRequests(cfg.TaskID)
	}
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			if cfg.TaskKey != "" {
				return nil, errors.WrapSentinel(errors.New(fmt.Sprintf("Artifacts for task key '%s' not found", cfg.TaskKey)), api.ErrNotFound)
			}
			return nil, errors.WrapSentinel(errors.New(fmt.Sprintf("Artifacts for task %s not found", cfg.TaskID)), api.ErrNotFound)
		}
		return nil, errors.Wrap(err, "unable to fetch artifact download requests")
	}

	artifactCount = len(artifactDownloadRequests)
	for _, req := range artifactDownloadRequests {
		totalBytes += req.SizeInBytes
	}

	if len(artifactDownloadRequests) == 0 {
		if !cfg.Json {
			fmt.Fprintf(s.Stdout, "No artifacts found for task %s\n", cfg.TaskID)
		}
		result := &DownloadAllArtifactsResult{}
		if cfg.Json {
			if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
				return nil, errors.Wrap(err, "unable to encode JSON output")
			}
		}
		return result, nil
	}

	collectionDir := cfg.Output
	if !cfg.OutputExplicitlySet {
		collectionDir = filepath.Join(cfg.Output, downloadAllArtifactsDirName(cfg))
	}
	if err := prepareDirectoryOutput(collectionDir); err != nil {
		return nil, errors.Wrapf(err, "unable to create output directory %s", collectionDir)
	}

	artifactDirs, err := artifactOutputDirs(collectionDir, artifactDownloadRequests)
	if err != nil {
		return nil, err
	}

	stopSpinner := Spin(
		fmt.Sprintf("Downloading %d artifact(s)...", len(artifactDownloadRequests)),
		s.StderrIsTTY,
		s.Stderr,
	)

	type downloadResult struct {
		index int
		bytes []byte
		err   error
	}

	results := make([]downloadResult, len(artifactDownloadRequests))
	var wg sync.WaitGroup
	for i, req := range artifactDownloadRequests {
		wg.Add(1)
		go func(idx int, r api.ArtifactDownloadRequestResult) {
			defer wg.Done()
			artifactBytes, err := s.APIClient.DownloadArtifact(r)
			results[idx] = downloadResult{index: idx, bytes: artifactBytes, err: err}
		}(i, req)
	}
	wg.Wait()
	stopSpinner()

	for _, r := range results {
		if r.err != nil {
			return nil, errors.Wrapf(r.err, "unable to download artifact %s", artifactDownloadRequests[r.index].Key)
		}
	}

	var allOutputFiles []string
	for i, req := range artifactDownloadRequests {
		artifactBytes := results[i].bytes
		shouldExtract := req.Kind == "file" || (req.Kind == "directory" && cfg.AutoExtract)
		artifactDir := artifactDirs[i]

		if shouldExtract {
			extractedFiles, err := extractTar(artifactBytes, artifactDir)
			if err != nil {
				return nil, errors.Wrapf(err, "unable to extract tar archive for artifact %s", req.Key)
			}

			if !cfg.Json && req.Kind == "directory" {
				fmt.Fprintf(s.Stdout, "Extracted %d file(s) to %s\n", len(extractedFiles), artifactDir)
			}

			allOutputFiles = append(allOutputFiles, extractedFiles...)
		} else {
			outputPath := filepath.Join(artifactDir, downloadFilename(req.Filename))
			if err := prepareFileOutput(outputPath); err != nil {
				return nil, err
			}

			if _, err := os.Stat(outputPath); err == nil {
				if !cfg.Json {
					fmt.Fprintf(s.Stdout, "Overwriting existing file at %s\n", outputPath)
				}
			}

			if err := os.WriteFile(outputPath, artifactBytes, 0644); err != nil {
				return nil, errors.Wrapf(err, "unable to write artifact file to %s", outputPath)
			}

			allOutputFiles = append(allOutputFiles, outputPath)
		}
	}

	if cfg.Open {
		for _, file := range allOutputFiles {
			if err := open.Run(file); err != nil {
				if !cfg.Json {
					fmt.Fprintf(s.Stderr, "Failed to open %s: %v\n", file, err)
				}
			}
		}
	}

	result := &DownloadAllArtifactsResult{OutputFiles: allOutputFiles}

	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		if len(allOutputFiles) == 1 {
			fmt.Fprintf(s.Stdout, "Artifact downloaded to %s\n", allOutputFiles[0])
		} else {
			fmt.Fprintf(s.Stdout, "Downloaded %d artifact(s):\n", len(artifactDownloadRequests))
			for _, file := range allOutputFiles {
				fmt.Fprintf(s.Stdout, "  %s\n", file)
			}
		}
	}

	return result, nil
}

func artifactOutputDirs(collectionDir string, requests []api.ArtifactDownloadRequestResult) ([]string, error) {
	dirs := make([]string, len(requests))
	usedNames := map[string]int{}

	for i, req := range requests {
		baseName := safePathComponent(req.Key, fmt.Sprintf("artifact-%d", i+1))
		usedNames[baseName]++

		name := baseName
		if usedNames[baseName] > 1 {
			name = fmt.Sprintf("%s-%d", baseName, usedNames[baseName])
		}

		dir := filepath.Join(collectionDir, name)
		if err := prepareDirectoryOutput(dir); err != nil {
			return nil, errors.Wrapf(err, "unable to create output directory %s", dir)
		}
		dirs[i] = dir
	}

	return dirs, nil
}

func extractTar(data []byte, destDir string) ([]string, error) {
	tarReader := tar.NewReader(bytes.NewReader(data))

	var extractedFiles []string

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.Wrap(err, "unable to read tar header")
		}

		filePath := filepath.Join(destDir, header.Name)
		cleanedDestDir := filepath.Clean(destDir)
		cleanedFilePath := filepath.Clean(filePath)
		// Allow the destDir itself or anything under it, but block path traversal
		if cleanedFilePath != cleanedDestDir && !strings.HasPrefix(cleanedFilePath, cleanedDestDir+string(os.PathSeparator)) {
			return nil, fmt.Errorf("invalid file path in tar: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(filePath, 0755); err != nil {
				return nil, errors.Wrapf(err, "unable to create directory %s", filePath)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
				return nil, errors.Wrapf(err, "unable to create directory for %s", filePath)
			}

			outFile, err := os.Create(filePath)
			if err != nil {
				return nil, errors.Wrapf(err, "unable to create file %s", filePath)
			}

			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return nil, errors.Wrapf(err, "unable to extract file %s", filePath)
			}
			outFile.Close()

			if err := os.Chmod(filePath, os.FileMode(header.Mode)); err != nil {
				return nil, errors.Wrapf(err, "unable to set permissions for %s", filePath)
			}

			extractedFiles = append(extractedFiles, filePath)
		}
	}

	return extractedFiles, nil
}

func extractSingleFileTar(data []byte, outputPath string, artifactKey string) ([]string, error) {
	tarReader := tar.NewReader(bytes.NewReader(data))

	var fileBytes bytes.Buffer
	var fileMode int64 = 0644
	fileCount := 0

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.Wrap(err, "unable to read tar header")
		}

		switch header.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			fileCount++
			if fileCount > 1 {
				return nil, fmt.Errorf("expected file artifact %s to contain exactly one file, found %d", artifactKey, fileCount)
			}

			fileMode = header.Mode
			if _, err := io.Copy(&fileBytes, tarReader); err != nil {
				return nil, errors.Wrapf(err, "unable to extract file %s", header.Name)
			}
		}
	}

	if fileCount != 1 {
		return nil, fmt.Errorf("expected file artifact %s to contain exactly one file, found %d", artifactKey, fileCount)
	}

	if err := prepareFileOutput(outputPath); err != nil {
		return nil, err
	}

	if err := os.WriteFile(outputPath, fileBytes.Bytes(), os.FileMode(fileMode)); err != nil {
		return nil, errors.Wrapf(err, "unable to write artifact file to %s", outputPath)
	}

	return []string{outputPath}, nil
}
