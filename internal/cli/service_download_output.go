package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rwx-cloud/rwx/internal/errors"
)

func prepareFileOutput(path string) error {
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			return fmt.Errorf("output path %s is a directory, expected a file", path)
		}
	} else if !os.IsNotExist(err) {
		return errors.Wrapf(err, "unable to inspect output path %s", path)
	}

	outputDir := filepath.Dir(path)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return errors.Wrapf(err, "unable to create output directory %s", outputDir)
	}

	return nil
}

func prepareDirectoryOutput(path string) error {
	if info, err := os.Stat(path); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("output path %s is a file, expected a directory", path)
		}
	} else if os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0755); err != nil {
			return err
		}
	} else {
		return errors.Wrapf(err, "unable to inspect output path %s", path)
	}

	return nil
}

func downloadFilename(filename string) string {
	base := filepath.Base(filename)
	if base == "." || base == string(os.PathSeparator) {
		return "download"
	}
	return base
}

func artifactStem(filename string) string {
	return safePathComponent(strings.TrimSuffix(downloadFilename(filename), ".tar"), "artifact")
}

func safePathComponent(name string, fallback string) string {
	component := strings.TrimSpace(strings.NewReplacer("/", "_", "\\", "_").Replace(name))
	component = strings.Trim(component, ".")
	if component == "" {
		return fallback
	}
	return component
}
