package config

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rwx-cloud/rwx/internal/errors"
)

type FileBackend struct {
	PrimaryDirectory    string
	FallbackDirectories []string
}

const (
	fileBackendDirMode  fs.FileMode = 0o700
	fileBackendFileMode fs.FileMode = 0o600
)

func NewFileBackend(dirs []string) (*FileBackend, error) {
	if len(dirs) < 1 {
		return nil, fmt.Errorf("at least one directory must be provided")
	}

	primaryDirectory, err := expandTilde(dirs[0])
	if err != nil {
		return nil, err
	}

	fallbackDirectories := make([]string, len(dirs)-1)
	for i, dir := range dirs[1:] {
		fallbackDir, err := expandTilde(dir)
		if err != nil {
			return nil, err
		}
		fallbackDirectories[i] = fallbackDir
	}

	return &FileBackend{
		PrimaryDirectory:    primaryDirectory,
		FallbackDirectories: fallbackDirectories,
	}, nil
}

func (f FileBackend) Get(filename string) (string, error) {
	value, err := f.getFrom(f.PrimaryDirectory, filename)

	if err != nil && errors.Is(err, fs.ErrNotExist) {
		for _, dir := range f.FallbackDirectories {
			value, err = f.getFrom(dir, filename)

			if err != nil && errors.Is(err, fs.ErrNotExist) {
				continue
			}

			if err != nil {
				return value, err
			}

			if err := f.Set(filename, value); err != nil {
				return "", errors.Wrapf(err, "unable to migrate %q from %q to %q", filename, dir, f.PrimaryDirectory)
			}

			return value, nil
		}

		return "", nil
	}

	return value, err
}

func (f FileBackend) getFrom(dir, filename string) (string, error) {
	path := filepath.Join(dir, filename)
	fd, err := os.Open(path)
	if err != nil {
		return "", errors.Wrapf(err, "unable to open %q", path)
	}
	defer fd.Close()

	contents, err := io.ReadAll(fd)
	if err != nil {
		return "", errors.Wrapf(err, "error reading %q", path)
	}

	return strings.TrimSpace(string(contents)), nil
}

func (f FileBackend) Set(filename, value string) error {
	err := os.MkdirAll(f.PrimaryDirectory, fileBackendDirMode)
	if err != nil {
		return errors.Wrapf(err, "unable to create %q", f.PrimaryDirectory)
	}
	if err := os.Chmod(f.PrimaryDirectory, fileBackendDirMode); err != nil {
		return errors.Wrapf(err, "unable to chmod %q", f.PrimaryDirectory)
	}

	path := filepath.Join(f.PrimaryDirectory, filename)
	fd, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fileBackendFileMode)
	if err != nil {
		return errors.Wrapf(err, "unable to create %q", path)
	}
	defer fd.Close()
	if err := fd.Chmod(fileBackendFileMode); err != nil {
		return errors.Wrapf(err, "unable to chmod %q", path)
	}

	_, err = io.WriteString(fd, value)
	if err != nil {
		return errors.Wrapf(err, "unable to write to %q", path)
	}

	return nil
}

var tildeSlash = fmt.Sprintf("~%v", string(os.PathSeparator))

func expandTilde(dir string) (string, error) {
	if dir != "~" && !strings.HasPrefix(dir, tildeSlash) {
		return dir, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	if dir == "~" {
		return homeDir, nil
	}

	return filepath.Join(homeDir, strings.TrimPrefix(dir, tildeSlash)), nil
}
