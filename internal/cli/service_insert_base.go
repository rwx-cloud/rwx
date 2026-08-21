package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rwx-cloud/rwx/internal/errors"

	"github.com/goccy/go-yaml"
)

type InsertBaseConfig struct {
	RwxDirectory string
	Files        []string
	Json         bool
}

func (c InsertBaseConfig) Validate() error {
	return nil
}

type BaseSpec struct {
	Image  string `yaml:"image"`
	Config string `yaml:"config"`
	Arch   string `yaml:"arch"`
}

type BaseLayerRunFile struct {
	ResolvedBase BaseSpec
	OriginalPath string
	Error        error
}

type InsertDefaultBaseResult struct {
	ErroredRunFiles []BaseLayerRunFile
	UpdatedRunFiles []BaseLayerRunFile
}

func (r InsertDefaultBaseResult) HasChanges() bool {
	return len(r.ErroredRunFiles) > 0 || len(r.UpdatedRunFiles) > 0
}

func (s Service) InsertBase(cfg InsertBaseConfig) (InsertDefaultBaseResult, error) {
	err := cfg.Validate()
	if err != nil {
		return InsertDefaultBaseResult{}, errors.Wrap(err, "validation failed")
	}

	rwxDirectoryPath, err := findAndValidateRwxDirectoryPath(cfg.RwxDirectory)
	if err != nil {
		return InsertDefaultBaseResult{}, errors.Wrap(err, "unable to find .rwx directory")
	}

	yamlFiles, err := getFileOrDirectoryYAMLEntries(cfg.Files, rwxDirectoryPath)
	if err != nil {
		return InsertDefaultBaseResult{}, err
	}

	if len(yamlFiles) == 0 {
		return InsertDefaultBaseResult{}, fmt.Errorf("no files provided, and no yaml files found in directory %s", rwxDirectoryPath)
	}

	result, err := s.insertOrUpdateBase(yamlFiles, true)
	if err != nil {
		return InsertDefaultBaseResult{}, err
	}

	if cfg.Json {
		updatedBases := make(map[string]string)
		for _, runFile := range result.UpdatedRunFiles {
			updatedBases[relativePathFromWd(runFile.OriginalPath)] = runFile.ResolvedBase.Image
		}
		erroredBases := make(map[string]string)
		for _, runFile := range result.ErroredRunFiles {
			erroredBases[relativePathFromWd(runFile.OriginalPath)] = runFile.Error.Error()
		}
		output := struct {
			UpdatedBases map[string]string
			ErroredBases map[string]string `json:",omitempty"`
		}{
			UpdatedBases: updatedBases,
			ErroredBases: erroredBases,
		}
		if err := json.NewEncoder(s.Stdout).Encode(output); err != nil {
			return InsertDefaultBaseResult{}, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		if len(yamlFiles) == 0 {
			fmt.Fprintf(s.Stdout, "No run files found in %q.\n", cfg.RwxDirectory)
		} else if !result.HasChanges() {
			fmt.Fprintln(s.Stdout, "No run files needed base updates.")
		} else {
			if len(result.UpdatedRunFiles) > 0 {
				fmt.Fprintln(s.Stdout, "Updated base in the following run definitions:")
				for _, runFile := range result.UpdatedRunFiles {
					fmt.Fprintf(s.Stdout, "\t%s → %s\n", relativePathFromWd(runFile.OriginalPath), runFile.ResolvedBase.Image)
				}
				if len(result.ErroredRunFiles) > 0 {
					fmt.Fprintln(s.Stdout)
				}
			}

			if len(result.ErroredRunFiles) > 0 {
				fmt.Fprintln(s.Stdout, "Failed to update base in the following run definitions:")
				for _, runFile := range result.ErroredRunFiles {
					fmt.Fprintf(s.Stdout, "\t%s → %s\n", relativePathFromWd(runFile.OriginalPath), runFile.Error)
				}
			}
		}
	}

	return result, nil
}

func (s Service) insertDefaultBaseIfMissing(mintFiles []RwxDirectoryEntry) (InsertDefaultBaseResult, error) {
	return s.insertOrUpdateBase(mintFiles, false)
}

func (s Service) insertOrUpdateBase(mintFiles []RwxDirectoryEntry, updateDeprecated bool) (InsertDefaultBaseResult, error) {
	runFilesToInsert, err := s.getFilesForBaseInsert(mintFiles)
	if err != nil {
		return InsertDefaultBaseResult{}, err
	}

	var runFilesToUpdate []BaseLayerRunFile
	if updateDeprecated {
		runFilesToUpdate, err = s.getFilesForBaseUpdate(mintFiles)
		if err != nil {
			return InsertDefaultBaseResult{}, err
		}
	}

	if len(runFilesToInsert) == 0 && len(runFilesToUpdate) == 0 {
		return InsertDefaultBaseResult{}, nil
	}

	defaultBaseSpec, err := s.getDefaultBaseSpec()
	if err != nil {
		return InsertDefaultBaseResult{}, errors.Wrap(err, "unable to get default base spec")
	}

	erroredRunFiles := make([]BaseLayerRunFile, 0)
	updatedRunFiles := make([]BaseLayerRunFile, 0)

	// Insert base for files missing it
	for _, runFile := range runFilesToInsert {
		runFile.ResolvedBase = defaultBaseSpec

		err := s.writeRunFileWithBase(runFile)
		if err != nil {
			runFile.Error = err
			erroredRunFiles = append(erroredRunFiles, runFile)
		} else {
			updatedRunFiles = append(updatedRunFiles, runFile)
		}
	}

	// Update deprecated base configurations
	if updateDeprecated {
		updateResult, err := s.updateDeprecatedBase(runFilesToUpdate, defaultBaseSpec)
		if err != nil {
			return InsertDefaultBaseResult{}, err
		}

		erroredRunFiles = append(erroredRunFiles, updateResult.ErroredRunFiles...)
		updatedRunFiles = append(updatedRunFiles, updateResult.UpdatedRunFiles...)
	}

	return InsertDefaultBaseResult{
		ErroredRunFiles: erroredRunFiles,
		UpdatedRunFiles: updatedRunFiles,
	}, nil
}

func (s Service) getFilesForBaseInsert(entries []RwxDirectoryEntry) ([]BaseLayerRunFile, error) {
	yamlFiles := filterYAMLFilesForModification(entries, func(doc *YAMLDoc) bool {
		if !doc.HasTasks() {
			return false
		}

		// Skip files that already define a 'base'
		if doc.HasBase() {
			return false
		}

		// Skip package definitions; bases are not valid in them
		if doc.HasPackage() {
			return false
		}

		// Skip if all tasks in this file are embedded runs
		if doc.AllTasksAreEmbeddedRuns() {
			return false
		}

		return true
	})

	runFiles := make([]BaseLayerRunFile, 0)
	for _, yamlFile := range yamlFiles {
		runFiles = append(runFiles, BaseLayerRunFile{OriginalPath: yamlFile.Entry.OriginalPath})
	}

	return runFiles, nil
}

func (s Service) getDefaultBaseSpec() (BaseSpec, error) {
	result, err := s.APIClient.GetDefaultBase()

	if err != nil {
		return BaseSpec{}, errors.Wrap(err, "unable to get default base")
	}

	return BaseSpec{
		Image:  result.Image,
		Config: result.Config,
		Arch:   result.Arch,
	}, nil
}

func (s Service) writeRunFileWithBase(runFile BaseLayerRunFile) error {
	doc, err := ParseYAMLFile(runFile.OriginalPath)
	if err != nil {
		return err
	}

	resolvedBase := runFile.ResolvedBase
	base := yaml.MapSlice{
		{Key: "image", Value: resolvedBase.Image},
		{Key: "config", Value: resolvedBase.Config},
	}

	if resolvedBase.Arch != "" && resolvedBase.Arch != DefaultArch {
		base = append(base, yaml.MapItem{Key: "arch", Value: resolvedBase.Arch})
	}

	err = doc.InsertBefore("$.tasks", map[string]any{
		"base": base,
	})
	if err != nil {
		return err
	}

	return doc.WriteFile(runFile.OriginalPath)
}

func (s Service) getFilesForBaseUpdate(entries []RwxDirectoryEntry) ([]BaseLayerRunFile, error) {
	yamlFiles := filterYAMLFilesForModification(entries, func(doc *YAMLDoc) bool {
		if !doc.HasBase() {
			return false
		}

		// Include files that have deprecated os or tag fields
		return doc.HasBaseOs() || doc.HasBaseTag()
	})

	runFiles := make([]BaseLayerRunFile, 0)
	for _, yamlFile := range yamlFiles {
		runFiles = append(runFiles, BaseLayerRunFile{OriginalPath: yamlFile.Entry.OriginalPath})
	}

	return runFiles, nil
}

func (s Service) updateDeprecatedBase(runFiles []BaseLayerRunFile, defaultBaseSpec BaseSpec) (InsertDefaultBaseResult, error) {
	erroredRunFiles := make([]BaseLayerRunFile, 0, len(runFiles))
	updatedRunFiles := make([]BaseLayerRunFile, 0, len(runFiles))

	for _, runFile := range runFiles {
		resolvedBase, err := s.updateRunFileBase(runFile, defaultBaseSpec)
		if err != nil {
			runFile.Error = err
			erroredRunFiles = append(erroredRunFiles, runFile)
		} else {
			runFile.ResolvedBase = resolvedBase
			updatedRunFiles = append(updatedRunFiles, runFile)
		}
	}

	return InsertDefaultBaseResult{
		ErroredRunFiles: erroredRunFiles,
		UpdatedRunFiles: updatedRunFiles,
	}, nil
}

func (s Service) updateRunFileBase(runFile BaseLayerRunFile, defaultBaseSpec BaseSpec) (BaseSpec, error) {
	doc, err := ParseYAMLFile(runFile.OriginalPath)
	if err != nil {
		return BaseSpec{}, err
	}

	// Read current base values
	osValue := doc.TryReadStringAtPath("$.base.os")
	archValue := doc.TryReadStringAtPath("$.base.arch")
	currentImage := doc.TryReadStringAtPath("$.base.image")

	// Determine the image value
	var image string
	if osValue != "" {
		// Convert "ubuntu 24.04" to "ubuntu:24.04"
		image = strings.Replace(osValue, " ", ":", 1)
	} else if currentImage != "" {
		image = currentImage
	} else {
		image = defaultBaseSpec.Image
	}

	// Use default config (tag is removed)
	config := defaultBaseSpec.Config

	// Build the new base section
	base := yaml.MapSlice{
		{Key: "image", Value: image},
		{Key: "config", Value: config},
	}

	// Preserve arch if present and not default
	if archValue != "" && archValue != DefaultArch {
		base = append(base, yaml.MapItem{Key: "arch", Value: archValue})
	}

	err = doc.ReplaceRootField("base", base)
	if err != nil {
		return BaseSpec{}, err
	}

	err = doc.WriteFile(runFile.OriginalPath)
	if err != nil {
		return BaseSpec{}, err
	}

	return BaseSpec{
		Image:  image,
		Config: config,
		Arch:   archValue,
	}, nil
}
