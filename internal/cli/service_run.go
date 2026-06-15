package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/rwx-cloud/rwx/internal/git"
)

// runDefinitionOutsideRwxDir reports whether the resolved run definition file
// lives outside the resolved rwx directory. If rwxDirectoryPath is empty (no
// .rwx directory configured or discovered) the file is considered outside.
func runDefinitionOutsideRwxDir(runDefinitionPath, rwxDirectoryPath string) (bool, error) {
	if rwxDirectoryPath == "" {
		return true, nil
	}

	absRunDef, err := filepath.Abs(runDefinitionPath)
	if err != nil {
		return false, err
	}
	absRwxDir, err := filepath.Abs(rwxDirectoryPath)
	if err != nil {
		return false, err
	}

	rel, err := filepath.Rel(absRwxDir, absRunDef)
	if err != nil {
		return true, nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return true, nil
	}
	return false, nil
}

// appendWorkflowUploadEntry adds a synthetic .workflow directory entry and a
// .workflow/<basename> file entry (sourced from runDef) to entries so the run
// definition is included in the uploaded rwx directory when it lives outside
// the resolved rwx directory. If .workflow/<basename> already exists in
// entries, the file is placed at .workflow/<contentHash>/<basename> instead,
// where contentHash is a short sha256 of the file contents (deterministic so
// repeated runs of the same file land at the same path).
func appendWorkflowUploadEntry(entries []RwxDirectoryEntry, runDef RwxDirectoryEntry) []RwxDirectoryEntry {
	hasWorkflowDir := false
	existingPaths := make(map[string]bool, len(entries))
	for _, e := range entries {
		existingPaths[e.Path] = true
		if e.Path == ".workflow" && e.IsDir() {
			hasWorkflowDir = true
		}
	}

	if !hasWorkflowDir {
		entries = append(entries, RwxDirectoryEntry{
			Type:        "dir",
			Path:        ".workflow",
			Permissions: 0o755,
		})
	}

	basename := filepath.Base(runDef.OriginalPath)
	filePath := ".workflow/" + basename

	if existingPaths[filePath] {
		sum := sha256.Sum256([]byte(runDef.FileContents))
		nestedDir := ".workflow/" + hex.EncodeToString(sum[:8])
		entries = append(entries, RwxDirectoryEntry{
			Type:        "dir",
			Path:        nestedDir,
			Permissions: 0o755,
		})
		filePath = nestedDir + "/" + basename
	}

	entries = append(entries, RwxDirectoryEntry{
		Type:         "file",
		Path:         filePath,
		Permissions:  runDef.Permissions,
		FileContents: runDef.FileContents,
	})

	return entries
}

type InitiateRunConfig struct {
	InitParameters map[string]string
	Json           bool
	RwxDirectory   string
	MintFilePath   string
	NoCache        bool
	TargetedTasks  []string
	Title          string
	GitBranch      string
	GitSha         string
	Patchable      bool
	CliState       string
}

func (c InitiateRunConfig) Validate() error {
	if c.MintFilePath == "" {
		return errors.New("the path to a run definition must be provided using the --file flag.")
	}

	return nil
}

// InitiateRun will connect to the Cloud API and start a new run in Mint.
func (s Service) InitiateRun(cfg InitiateRunConfig) (*api.InitiateRunResult, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	var rwxDirectory []RwxDirectoryEntry

	rwxDirectoryPath, err := findAndValidateRwxDirectoryPath(cfg.RwxDirectory)
	if err != nil {
		return nil, errors.Wrap(err, "unable to find .rwx directory")
	}

	runDefinitionPath, err := FindRunDefinitionFile(cfg.MintFilePath, rwxDirectoryPath)
	if err != nil {
		return nil, err
	}

	gitInstalled := s.GitClient.IsInstalled()
	gitDirectory := s.GitClient.IsInsideWorkTree()
	var errorMessage string
	var sha, branch, originUrl string
	var repository git.RepositoryMetadata

	// Track whether we can generate patches (requires working git)
	gitAvailable := gitInstalled && gitDirectory

	if gitAvailable {
		var err error
		sha, err = s.GitClient.GetCommit()
		if err != nil {
			errorMessage = err.Error()
			gitAvailable = false
		} else {
			branch = s.GitClient.GetBranch()
			originUrl = s.GitClient.GetOriginUrl()
			repository = git.RepositoryMetadataFromOriginUrl(originUrl)
		}
	} else if !gitInstalled {
		errorMessage = "Git is not installed"
	} else if !gitDirectory {
		errorMessage = "You are not in a git repository"
	}

	patchFile := git.PatchFile{}

	// When there's no .rwx directory, create a temporary one for patches and to set run.dir
	var tempRwxDir string
	if rwxDirectoryPath == "" {
		tempRwxDir, err = os.MkdirTemp("", ".rwx-*")
		if err != nil {
			return nil, errors.Wrap(err, "unable to create temporary .rwx directory")
		}
		defer os.RemoveAll(tempRwxDir)
		rwxDirectoryPath = tempRwxDir
	}

	patchDir := filepath.Join(rwxDirectoryPath, ".patches")
	defer os.RemoveAll(patchDir)

	// Generate patches if enabled and git is available
	patchable := cfg.Patchable && gitAvailable
	if _, ok := os.LookupEnv("RWX_DISABLE_GIT_PATCH"); ok {
		patchable = false
	}

	// Convert to relative path for display purposes (e.g., run title)
	relativeRunDefinitionPath := relativePathFromWd(runDefinitionPath)

	resolveResult, err := ResolveCliParamsForFile(relativeRunDefinitionPath)
	if err != nil {
		return nil, errors.Wrap(err, "unable to resolve CLI init params")
	}

	if resolveResult.Rewritten {
		fmt.Fprintf(s.Stderr, "Configured CLI trigger with git init params in %q\n\n", relativeRunDefinitionPath)
	}

	for _, gitParam := range resolveResult.GitParams {
		if _, exists := cfg.InitParameters[gitParam]; exists {
			patchable = false
			break
		}
	}

	if patchable {
		var patchErr error
		patchFile, patchErr = s.GitClient.GeneratePatchFile(patchDir, []string{".", ":!" + relativeRunDefinitionPath})
		if patchErr != nil {
			errorMessage = patchErr.Error()
			fmt.Fprintf(s.Stderr, "Warning: %s\n\n", errorMessage)

			// Telemetry gets a stable, PII-free classification — never raw git
			// stderr, which embeds customer file paths, branch names, and layout.
			telemetryProps := map[string]any{
				"failed_command": "unknown",
				"exit_code":      -1,
				"reason":         "unknown",
			}
			var pe *git.PatchError
			if errors.As(patchErr, &pe) {
				telemetryProps["failed_command"] = pe.Command
				telemetryProps["exit_code"] = pe.ExitCode
				telemetryProps["reason"] = pe.Reason()
			}
			s.recordTelemetry("run.patch_error", telemetryProps)
		}
		if patchFile.LFSChangedFiles.Count > 0 {
			return nil, runPatchLFSChangesError(patchFile.LFSChangedFiles)
		}
	}

	// Load directory entries
	entries, err := rwxDirectoryEntries(rwxDirectoryPath)
	if err != nil {
		if errors.Is(err, errors.ErrFileNotExists) && tempRwxDir == "" {
			// User explicitly specified a directory that doesn't exist
			return nil, fmt.Errorf("You specified --dir %q, but %q could not be found", cfg.RwxDirectory, cfg.RwxDirectory)
		}

		return nil, errors.Wrapf(err, "unable to load directory %q", rwxDirectoryPath)
	}

	rwxDirectory = entries

	runDefinition, err := rwxDirectoryEntriesFromPaths([]string{relativeRunDefinitionPath})
	if err != nil {
		return nil, errors.Wrap(err, "unable to read provided files")
	}
	runDefinition = filterFiles(runDefinition)
	if len(runDefinition) != 1 {
		return nil, fmt.Errorf("expected exactly 1 run definition, got %d", len(runDefinition))
	}

	// reloadRunDefinitions reloads run definitions after modifying the file.
	reloadRunDefinitions := func() error {
		runDefinition, err = rwxDirectoryEntriesFromPaths([]string{relativeRunDefinitionPath})
		if err != nil {
			return errors.Wrapf(err, "unable to reload %q", relativeRunDefinitionPath)
		}
		rwxDirectoryEntries, err := rwxDirectoryEntries(rwxDirectoryPath)
		if err != nil && !errors.Is(err, errors.ErrFileNotExists) {
			return errors.Wrapf(err, "unable to reload rwx directory %q", rwxDirectoryPath)
		}

		rwxDirectory = rwxDirectoryEntries
		return nil
	}

	if patchFile.Written {
		fmt.Fprintf(s.Stderr, "Included a git patch for uncommitted changes\n")
		fmt.Fprintln(s.Stderr, "")
	}

	addBaseIfNeeded, err := s.insertDefaultBaseIfMissing(runDefinition)
	if err != nil {
		return nil, errors.Wrap(err, "unable to resolve base")
	}

	if len(addBaseIfNeeded.UpdatedRunFiles) > 0 {
		update := addBaseIfNeeded.UpdatedRunFiles[0]
		fmt.Fprintf(s.Stderr, "Configured %q to run on %s\n\n", update.OriginalPath, update.ResolvedBase.Image)

		if err = reloadRunDefinitions(); err != nil {
			return nil, err
		}
	}

	if len(addBaseIfNeeded.ErroredRunFiles) > 0 {
		for _, erroredFile := range addBaseIfNeeded.ErroredRunFiles {
			fmt.Fprintf(s.Stderr, "Failed to configure base for %q: %v\n", erroredFile.OriginalPath, erroredFile.Error)
		}
	}

	mintFiles := filterYAMLFilesForModification(runDefinition, func(doc *YAMLDoc) bool {
		return true
	})
	resolvedPackages, err := s.resolveOrUpdatePackagesForFiles(mintFiles, false, PickLatestMajorVersion)
	if err != nil {
		return nil, err
	}
	if len(resolvedPackages) > 0 {
		for rwxPackage, version := range resolvedPackages {
			fmt.Fprintf(s.Stderr, "Configured package %s to use version %s\n", rwxPackage, version)
		}
		fmt.Fprintln(s.Stderr, "")

		if err = reloadRunDefinitions(); err != nil {
			return nil, err
		}
	}

	if outside, err := runDefinitionOutsideRwxDir(runDefinitionPath, rwxDirectoryPath); err == nil && outside {
		rwxDirectory = appendWorkflowUploadEntry(rwxDirectory, runDefinition[0])
	}

	i := 0
	initializationParameters := make([]api.InitializationParameter, len(cfg.InitParameters))
	for key, value := range cfg.InitParameters {
		initializationParameters[i] = api.InitializationParameter{
			Key:   key,
			Value: value,
		}
		i++
	}

	initiateStart := time.Now()
	runResult, err := s.APIClient.InitiateRun(api.InitiateRunConfig{
		InitializationParameters: initializationParameters,
		TaskDefinitions:          runDefinition,
		RwxDirectory:             rwxDirectory,
		TargetedTaskKeys:         cfg.TargetedTasks,
		Title:                    cfg.Title,
		UseCache:                 !cfg.NoCache,
		CliState:                 cfg.CliState,
		Git: api.GitMetadata{
			Branch:    branch,
			Sha:       sha,
			OriginUrl: originUrl,
		},
		RepositoryName: repository.Name,
		RepositorySlug: repository.Slug,
		RepositoryURL:  repository.URL,
		VCSProvider:    repository.VCSProvider,
		Patch: api.PatchMetadata{
			Sent:           patchFile.Written,
			UntrackedFiles: []string{},
			UntrackedCount: 0,
			LFSFiles:       patchFile.LFSChangedFiles.Files,
			LFSCount:       patchFile.LFSChangedFiles.Count,
			ErrorMessage:   errorMessage,
			GitDirectory:   gitDirectory,
			GitInstalled:   gitInstalled,
		},
	})

	s.recordTelemetry("run.initiate", map[string]any{
		"has_targets":     len(cfg.TargetedTasks) > 0,
		"has_init_params": len(cfg.InitParameters) > 0,
		"duration_ms":     time.Since(initiateStart).Milliseconds(),
		"success":         err == nil,
	})

	if err != nil {
		return nil, errors.Wrap(err, "Failed to initiate run")
	}

	return runResult, nil
}
