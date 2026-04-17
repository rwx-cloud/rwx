package cli

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	tsize "github.com/kopoli/go-terminal-size"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/rwx-cloud/rwx/internal/text"

	"github.com/goccy/go-yaml/ast"
)

type UpdatePackagesConfig struct {
	RwxDirectory             string
	Files                    []string
	ReplacementVersionPicker func(versions api.PackageVersionsResult, rwxPackage string, major string) (string, error)
	Json                     bool
}

func (c UpdatePackagesConfig) Validate() error {
	if c.ReplacementVersionPicker == nil {
		return errors.New("a replacement version picker must be provided")
	}

	return nil
}

type ResolvePackagesConfig struct {
	RwxDirectory        string
	Files               []string
	LatestVersionPicker func(versions api.PackageVersionsResult, rwxPackage string, _ string) (string, error)
	Json                bool
}

func (c ResolvePackagesConfig) PickLatestVersion(versions api.PackageVersionsResult, rwxPackage string) (string, error) {
	return c.LatestVersionPicker(versions, rwxPackage, "")
}

func (c ResolvePackagesConfig) Validate() error {
	if c.LatestVersionPicker == nil {
		return errors.New("a latest version picker must be provided")
	}

	return nil
}

type UpdatePackagesResult struct {
	UpdatedPackages map[string]string
}

type ResolvePackagesResult struct {
	ResolvedPackages map[string]string
}

func (r ResolvePackagesResult) HasChanges() bool {
	return len(r.ResolvedPackages) > 0
}

func (s Service) ResolvePackages(cfg ResolvePackagesConfig) (ResolvePackagesResult, error) {
	err := cfg.Validate()
	if err != nil {
		return ResolvePackagesResult{}, errors.Wrap(err, "validation failed")
	}

	rwxDirectoryPath, err := findAndValidateRwxDirectoryPath(cfg.RwxDirectory)
	if err != nil {
		return ResolvePackagesResult{}, errors.Wrap(err, "unable to find .rwx directory")
	}

	yamlFiles, err := getFileOrDirectoryYAMLEntries(cfg.Files, rwxDirectoryPath)
	if err != nil {
		return ResolvePackagesResult{}, err
	}

	if len(yamlFiles) == 0 {
		return ResolvePackagesResult{}, fmt.Errorf("no files provided, and no yaml files found in directory %s", rwxDirectoryPath)
	}

	mintFiles := filterYAMLFilesForModification(yamlFiles, func(doc *YAMLDoc) bool {
		return true
	})

	replacements, err := s.resolveOrUpdatePackagesForFiles(mintFiles, false, cfg.LatestVersionPicker)
	if err != nil {
		return ResolvePackagesResult{}, err
	}

	if cfg.Json {
		output := struct {
			ResolvedPackages map[string]string
		}{
			ResolvedPackages: replacements,
		}
		if err := json.NewEncoder(s.Stdout).Encode(output); err != nil {
			return ResolvePackagesResult{}, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		if len(replacements) == 0 {
			fmt.Fprintln(s.Stdout, "No packages to resolve.")
		} else {
			fmt.Fprintln(s.Stdout, "Resolved the following packages:")
			for rwxPackage, version := range replacements {
				fmt.Fprintf(s.Stdout, "\t%s → %s\n", rwxPackage, version)
			}
		}
	}

	s.recordTelemetry("packages.resolve", map[string]any{
		"package_count": len(replacements),
	})

	return ResolvePackagesResult{ResolvedPackages: replacements}, nil
}

func (s Service) UpdatePackages(cfg UpdatePackagesConfig) (*UpdatePackagesResult, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	rwxDirectoryPath, err := findAndValidateRwxDirectoryPath(cfg.RwxDirectory)
	if err != nil {
		return nil, errors.Wrap(err, "unable to find .rwx directory")
	}

	yamlFiles, err := getFileOrDirectoryYAMLEntries(cfg.Files, rwxDirectoryPath)
	if err != nil {
		return nil, err
	}

	if len(yamlFiles) == 0 {
		return nil, errors.New(fmt.Sprintf("no files provided, and no yaml files found in directory %s", rwxDirectoryPath))
	}

	mintFiles := filterYAMLFilesForModification(yamlFiles, func(doc *YAMLDoc) bool {
		return true
	})

	replacements, err := s.resolveOrUpdatePackagesForFiles(mintFiles, true, cfg.ReplacementVersionPicker)
	if err != nil {
		return nil, err
	}

	s.recordTelemetry("packages.update", map[string]any{
		"package_count": len(replacements),
	})

	result := &UpdatePackagesResult{UpdatedPackages: replacements}

	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		if len(replacements) == 0 {
			fmt.Fprintln(s.Stdout, "All packages are up-to-date.")
		} else {
			fmt.Fprintln(s.Stdout, "Updated the following packages:")
			for original, replacement := range replacements {
				fmt.Fprintf(s.Stdout, "\t%s → %s\n", original, replacement)
			}
		}
	}

	return result, nil
}

var rePackageVersion = regexp.MustCompile(`([a-z0-9-]+\/[a-z0-9-]+)(?:\s+(([0-9]+)\.[0-9]+\.[0-9]+))?`)

type PackageVersion struct {
	Original     string
	Name         string
	Version      string
	MajorVersion string
}

func (s Service) parsePackageVersion(str string) PackageVersion {
	match := rePackageVersion.FindStringSubmatch(str)
	if len(match) == 0 {
		return PackageVersion{}
	}

	return PackageVersion{
		Original:     match[0],
		Name:         tryGetSliceAtIndex(match, 1, ""),
		Version:      tryGetSliceAtIndex(match, 2, ""),
		MajorVersion: tryGetSliceAtIndex(match, 3, ""),
	}
}

func (s Service) updatePackageReferenceAtPath(
	doc *YAMLDoc,
	yamlPath string,
	original string,
	update bool,
	packageVersions *api.PackageVersionsResult,
	versionPicker func(versions api.PackageVersionsResult, rwxPackage string, major string) (string, error),
	replacements map[string]string,
) (bool, error) {
	// Expressions can't be statically resolved, so skip them
	if strings.Contains(original, "${{") {
		return false, nil
	}

	packageVersion := s.parsePackageVersion(original)
	if packageVersion.Name == "" {
		return false, nil
	} else if !update && packageVersion.MajorVersion != "" {
		return false, nil
	}

	newName := packageVersions.Renames[packageVersion.Name]
	if newName == "" {
		newName = packageVersion.Name
	}

	targetPackageVersion, err := versionPicker(*packageVersions, newName, packageVersion.MajorVersion)
	if err != nil {
		fmt.Fprintln(s.Stderr, err.Error())
		return false, nil
	}

	newPackage := fmt.Sprintf("%s %s", newName, targetPackageVersion)
	if newPackage == original {
		return false, nil
	}

	if err := doc.ReplaceAtPath(yamlPath, newPackage); err != nil {
		return false, err
	}

	if newName != packageVersion.Name {
		replacements[packageVersion.Original] = fmt.Sprintf("%s %s", newName, targetPackageVersion)
	} else {
		replacements[packageVersion.Original] = targetPackageVersion
	}

	return true, nil
}

func (s Service) resolveOrUpdatePackagesForFiles(mintFiles []*MintYAMLFile, update bool, versionPicker func(versions api.PackageVersionsResult, rwxPackage string, major string) (string, error)) (map[string]string, error) {
	packageVersions, err := s.APIClient.GetPackageVersions()
	if err != nil {
		return nil, errors.Wrap(err, "unable to fetch package versions")
	}

	docs := make(map[string]*YAMLDoc)
	replacements := make(map[string]string)

	for _, file := range mintFiles {
		hasChange := false

		var nodePath string
		if file.Doc.IsRunDefinition() {
			nodePath = "$.tasks[*].call"
		} else if file.Doc.IsListOfTasks() {
			nodePath = "$[*].call"
		} else {
			continue
		}

		err = file.Doc.ForEachNode(nodePath, func(node ast.Node) error {
			changed, err := s.updatePackageReferenceAtPath(file.Doc, node.GetPath(), node.String(), update, packageVersions, versionPicker, replacements)
			if err != nil {
				return err
			}
			if changed {
				hasChange = true
			}
			return nil
		})
		if err != nil {
			return nil, errors.Wrap(err, "unable to replace package references")
		}

		if file.Doc.IsRunDefinition() {
			baseConfig := file.Doc.TryReadStringAtPath("$.base.config")
			if baseConfig != "" && baseConfig != "none" {
				changed, err := s.updatePackageReferenceAtPath(file.Doc, "$.base.config", baseConfig, update, packageVersions, versionPicker, replacements)
				if err != nil {
					return nil, errors.Wrap(err, "unable to replace base.config package reference")
				}
				if changed {
					hasChange = true
				}
			}
		}

		if hasChange {
			docs[file.Entry.OriginalPath] = file.Doc
		}
	}

	for path, doc := range docs {
		if !doc.HasChanges() {
			continue
		}

		err := doc.WriteFile(path)
		if err != nil {
			return replacements, err
		}
	}

	return replacements, nil
}

func PickLatestMajorVersion(versions api.PackageVersionsResult, rwxPackage string, _ string) (string, error) {
	latestVersion, ok := versions.LatestMajor[rwxPackage]
	if !ok {
		return "", fmt.Errorf("Unable to find the package %q; skipping it.", rwxPackage)
	}

	return latestVersion, nil
}

func PickLatestMinorVersion(versions api.PackageVersionsResult, rwxPackage string, major string) (string, error) {
	if major == "" {
		return PickLatestMajorVersion(versions, rwxPackage, major)
	}

	majorVersions, ok := versions.LatestMinor[rwxPackage]
	if !ok {
		return "", fmt.Errorf("Unable to find the package %q; skipping it.", rwxPackage)
	}

	latestVersion, ok := majorVersions[major]
	if !ok {
		return "", fmt.Errorf("Unable to find major version %q for package %q; skipping it.", major, rwxPackage)
	}

	return latestVersion, nil
}

type ListPackagesConfig struct {
	Json bool
}

type PackageInfo struct {
	Name          string
	Description   string
	LatestVersion string
	Versions      map[string]string
}

type ListPackagesResult struct {
	Packages []PackageInfo
}

func (s Service) ListPackages(cfg ListPackagesConfig) (*ListPackagesResult, error) {
	packageVersions, err := s.APIClient.GetPackageVersions()
	if err != nil {
		return nil, errors.Wrap(err, "unable to fetch package versions")
	}

	names := make([]string, 0, len(packageVersions.LatestMajor))
	for name := range packageVersions.LatestMajor {
		if _, isRenamed := packageVersions.Renames[name]; isRenamed {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	packages := make([]PackageInfo, 0, len(names))
	for _, name := range names {
		info := PackageInfo{
			Name:          name,
			LatestVersion: packageVersions.LatestMajor[name],
		}
		if pkgInfo, ok := packageVersions.Packages[name]; ok {
			info.Description = pkgInfo.Description
		}
		if minorVersions, ok := packageVersions.LatestMinor[name]; ok {
			info.Versions = minorVersions
		}
		packages = append(packages, info)
	}

	result := &ListPackagesResult{Packages: packages}

	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		if len(packages) == 0 {
			fmt.Fprintln(s.Stdout, "No packages found.")
		} else {
			maxNameLen := len("PACKAGE")
			maxVersionLen := len("LATEST")
			for _, pkg := range packages {
				if len(pkg.Name) > maxNameLen {
					maxNameLen = len(pkg.Name)
				}
				if len(pkg.LatestVersion) > maxVersionLen {
					maxVersionLen = len(pkg.LatestVersion)
				}
			}
			prefixWidth := maxNameLen + 2 + maxVersionLen + 2
			termWidth := 80
			if size, err := tsize.GetSize(); err == nil && size.Width > 0 {
				termWidth = size.Width
			}
			descWidth := termWidth - prefixWidth
			if !s.StdoutIsTTY || descWidth < 20 {
				descWidth = 0 // too narrow to wrap or non-TTY; print as-is
			}
			fmtStr := fmt.Sprintf("%%-%ds  %%-%ds  %%s\n", maxNameLen, maxVersionLen)
			wrapIndent := strings.Repeat(" ", prefixWidth)
			fmt.Fprintf(s.Stdout, fmtStr, "PACKAGE", "LATEST", "DESCRIPTION")
			for _, pkg := range packages {
				lines := text.WrapText(pkg.Description, descWidth)
				fmt.Fprintf(s.Stdout, fmtStr, pkg.Name, pkg.LatestVersion, lines[0])
				for _, line := range lines[1:] {
					fmt.Fprintf(s.Stdout, "%s%s\n", wrapIndent, line)
				}
			}
		}
	}

	return result, nil
}

type ShowPackageConfig struct {
	PackageName string
	Json        bool
	NoReadme    bool
}

type ShowPackageResult struct {
	Name            string
	Version         string
	Description     string
	SourceCodeUrl   string
	IssueTrackerUrl string
	Parameters      []api.PackageDocumentationParameter
	Readme          string
}

func (s Service) ShowPackage(cfg ShowPackageConfig) (*ShowPackageResult, error) {
	doc, err := s.APIClient.GetPackageDocumentation(cfg.PackageName)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("unable to fetch documentation for package %q", cfg.PackageName))
	}

	result := &ShowPackageResult{
		Name:            doc.Name,
		Version:         doc.Version,
		Description:     doc.Description,
		SourceCodeUrl:   doc.SourceCodeUrl,
		IssueTrackerUrl: doc.IssueTrackerUrl,
		Parameters:      doc.Parameters,
		Readme:          doc.Readme,
	}

	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		termWidth := 80
		if size, err := tsize.GetSize(); err == nil && size.Width > 0 {
			termWidth = size.Width
		}
		fmt.Fprintf(s.Stdout, "Name:         %s\n", doc.Name)
		fmt.Fprintf(s.Stdout, "Version:      %s\n", doc.Version)
		descWrapWidth := termWidth - 14
		if !s.StdoutIsTTY {
			descWrapWidth = 0
		}
		descLines := text.WrapText(doc.Description, descWrapWidth)
		fmt.Fprintf(s.Stdout, "Description:  %s\n", descLines[0])
		for _, line := range descLines[1:] {
			fmt.Fprintf(s.Stdout, "              %s\n", line)
		}
		if doc.SourceCodeUrl != "" {
			fmt.Fprintf(s.Stdout, "Source Code:  %s\n", doc.SourceCodeUrl)
		}
		if doc.IssueTrackerUrl != "" {
			fmt.Fprintf(s.Stdout, "Issues:       %s\n", doc.IssueTrackerUrl)
		}

		if len(doc.Parameters) > 0 {
			fmt.Fprintln(s.Stdout)
			maxParamNameLen := len("PARAMETER")
			maxDefaultLen := len("DEFAULT")
			for _, p := range doc.Parameters {
				if len(p.Name) > maxParamNameLen {
					maxParamNameLen = len(p.Name)
				}
				if p.Default != nil {
					if len(string(*p.Default)) > maxDefaultLen {
						maxDefaultLen = len(string(*p.Default))
					}
				}
			}
			// PARAMETER + gap + REQUIRED(8) + gap + DEFAULT + gap
			prefixWidth := maxParamNameLen + 2 + 8 + 2 + maxDefaultLen + 2
			descWidth := termWidth - prefixWidth
			if !s.StdoutIsTTY || descWidth < 20 {
				descWidth = 0 // too narrow to wrap or non-TTY; print as-is
			}
			paramFmt := fmt.Sprintf("%%-%ds  %%-8s  %%-%ds  %%s\n", maxParamNameLen, maxDefaultLen)
			wrapIndent := strings.Repeat(" ", prefixWidth)
			fmt.Fprintf(s.Stdout, paramFmt, "PARAMETER", "REQUIRED", "DEFAULT", "DESCRIPTION")
			for _, p := range doc.Parameters {
				required := "false"
				if p.Required {
					required = "true"
				}
				defaultVal := ""
				if p.Default != nil {
					defaultVal = string(*p.Default)
				}
				lines := text.WrapText(p.Description, descWidth)
				fmt.Fprintf(s.Stdout, paramFmt, p.Name, required, defaultVal, lines[0])
				for _, line := range lines[1:] {
					fmt.Fprintf(s.Stdout, "%s%s\n", wrapIndent, line)
				}
			}
		}

		if !cfg.NoReadme && doc.Readme != "" {
			fmt.Fprintln(s.Stdout)
			fmt.Fprint(s.Stdout, doc.Readme)
		}
	}

	return result, nil
}
