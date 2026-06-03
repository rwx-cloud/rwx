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

type runGitMetadata struct {
	installed    bool
	directory    bool
	available    bool
	errorMessage string
	sha          string
	branch       string
	originURL    string
}

func (s Service) runGitMetadata() runGitMetadata {
	metadata := runGitMetadata{
		installed: s.GitClient.IsInstalled(),
		directory: s.GitClient.IsInsideWorkTree(),
	}
	metadata.available = metadata.installed && metadata.directory

	if metadata.available {
		sha, err := s.GitClient.GetCommit()
		if err != nil {
			metadata.errorMessage = err.Error()
			metadata.available = false
		} else {
			metadata.sha = sha
			metadata.branch = s.GitClient.GetBranch()
			metadata.originURL = s.GitClient.GetOriginUrl()
		}
	} else if !metadata.installed {
		metadata.errorMessage = "Git is not installed"
	} else if !metadata.directory {
		metadata.errorMessage = "You are not in a git repository"
	}

	return metadata
}

func (m runGitMetadata) apiGitMetadata() api.GitMetadata {
	return api.GitMetadata{
		Branch:    m.branch,
		Sha:       m.sha,
		OriginUrl: m.originURL,
	}
}

func (m runGitMetadata) apiPatchMetadata(patchFile git.PatchFile) api.PatchMetadata {
	return api.PatchMetadata{
		Sent:           patchFile.Written,
		UntrackedFiles: patchFile.UntrackedFiles.Files,
		UntrackedCount: patchFile.UntrackedFiles.Count,
		LFSFiles:       patchFile.LFSChangedFiles.Files,
		LFSCount:       patchFile.LFSChangedFiles.Count,
		ErrorMessage:   m.errorMessage,
		GitDirectory:   m.directory,
		GitInstalled:   m.installed,
	}
}

type runDefinitionPaths struct {
	rwxDirectoryPath          string
	runDefinitionPath         string
	relativeRunDefinitionPath string
	tempRwxDir                string
}

func resolveRunDefinitionPaths(rwxDirectory, mintFilePath string) (runDefinitionPaths, func(), error) {
	rwxDirectoryPath, err := findAndValidateRwxDirectoryPath(rwxDirectory)
	if err != nil {
		return runDefinitionPaths{}, func() {}, errors.Wrap(err, "unable to find .rwx directory")
	}

	runDefinitionPath, err := FindRunDefinitionFile(mintFilePath, rwxDirectoryPath)
	if err != nil {
		return runDefinitionPaths{}, func() {}, err
	}

	paths := runDefinitionPaths{
		rwxDirectoryPath:          rwxDirectoryPath,
		runDefinitionPath:         runDefinitionPath,
		relativeRunDefinitionPath: relativePathFromWd(runDefinitionPath),
	}

	if paths.rwxDirectoryPath == "" {
		tempRwxDir, err := os.MkdirTemp("", ".rwx-*")
		if err != nil {
			return runDefinitionPaths{}, func() {}, errors.Wrap(err, "unable to create temporary .rwx directory")
		}
		paths.rwxDirectoryPath = tempRwxDir
		paths.tempRwxDir = tempRwxDir
		return paths, func() { os.RemoveAll(tempRwxDir) }, nil
	}

	return paths, func() {}, nil
}

type runDefinitionUpload struct {
	paths         runDefinitionPaths
	rwxDirectory  []RwxDirectoryEntry
	runDefinition []RwxDirectoryEntry
}

func loadRunDefinitionUpload(paths runDefinitionPaths, requestedRwxDirectory string) (*runDefinitionUpload, error) {
	upload := &runDefinitionUpload{paths: paths}
	if err := upload.load(requestedRwxDirectory); err != nil {
		return nil, err
	}
	return upload, nil
}

func (u *runDefinitionUpload) load(requestedRwxDirectory string) error {
	entries, err := rwxDirectoryEntries(u.paths.rwxDirectoryPath)
	if err != nil {
		if errors.Is(err, errors.ErrFileNotExists) && u.paths.tempRwxDir == "" {
			return fmt.Errorf("You specified --dir %q, but %q could not be found", requestedRwxDirectory, requestedRwxDirectory)
		}

		return errors.Wrapf(err, "unable to load directory %q", u.paths.rwxDirectoryPath)
	}
	u.rwxDirectory = entries

	runDefinition, err := rwxDirectoryEntriesFromPaths([]string{u.paths.relativeRunDefinitionPath})
	if err != nil {
		return errors.Wrap(err, "unable to read provided files")
	}
	runDefinition = filterFiles(runDefinition)
	if len(runDefinition) != 1 {
		return fmt.Errorf("expected exactly 1 run definition, got %d", len(runDefinition))
	}
	u.runDefinition = runDefinition

	return nil
}

func (u *runDefinitionUpload) reload() error {
	runDefinition, err := rwxDirectoryEntriesFromPaths([]string{u.paths.relativeRunDefinitionPath})
	if err != nil {
		return errors.Wrapf(err, "unable to reload %q", u.paths.relativeRunDefinitionPath)
	}
	u.runDefinition = filterFiles(runDefinition)

	rwxDirectory, err := rwxDirectoryEntries(u.paths.rwxDirectoryPath)
	if err != nil && !errors.Is(err, errors.ErrFileNotExists) {
		return errors.Wrapf(err, "unable to reload rwx directory %q", u.paths.rwxDirectoryPath)
	}
	u.rwxDirectory = rwxDirectory

	return nil
}

func (u *runDefinitionUpload) appendRunDefinitionOutsideRwxDir() {
	if outside, err := runDefinitionOutsideRwxDir(u.paths.runDefinitionPath, u.paths.rwxDirectoryPath); err == nil && outside {
		u.rwxDirectory = appendWorkflowUploadEntry(u.rwxDirectory, u.runDefinition[0])
	}
}

func (s Service) cliInitParamsOverrideGitParams(relativeRunDefinitionPath string, initParams map[string]string) (bool, error) {
	resolveResult, err := ResolveCliParamsForFile(relativeRunDefinitionPath)
	if err != nil {
		return false, errors.Wrap(err, "unable to resolve CLI init params")
	}

	if resolveResult.Rewritten {
		fmt.Fprintf(s.Stderr, "Configured CLI trigger with git init params in %q\n\n", relativeRunDefinitionPath)
	}

	for _, gitParam := range resolveResult.GitParams {
		if _, exists := initParams[gitParam]; exists {
			return true, nil
		}
	}

	return false, nil
}

func (s Service) configureRunDefinitionUpload(upload *runDefinitionUpload) error {
	addBaseIfNeeded, err := s.insertDefaultBaseIfMissing(upload.runDefinition)
	if err != nil {
		return errors.Wrap(err, "unable to resolve base")
	}

	if len(addBaseIfNeeded.UpdatedRunFiles) > 0 {
		update := addBaseIfNeeded.UpdatedRunFiles[0]
		fmt.Fprintf(s.Stderr, "Configured %q to run on %s\n\n", update.OriginalPath, update.ResolvedBase.Image)

		if err = upload.reload(); err != nil {
			return err
		}
	}

	if len(addBaseIfNeeded.ErroredRunFiles) > 0 {
		for _, erroredFile := range addBaseIfNeeded.ErroredRunFiles {
			fmt.Fprintf(s.Stderr, "Failed to configure base for %q: %v\n", erroredFile.OriginalPath, erroredFile.Error)
		}
	}

	mintFiles := filterYAMLFilesForModification(upload.runDefinition, func(doc *YAMLDoc) bool {
		return true
	})
	resolvedPackages, err := s.resolveOrUpdatePackagesForFiles(mintFiles, false, PickLatestMajorVersion)
	if err != nil {
		return err
	}
	if len(resolvedPackages) > 0 {
		for rwxPackage, version := range resolvedPackages {
			fmt.Fprintf(s.Stderr, "Configured package %s to use version %s\n", rwxPackage, version)
		}
		fmt.Fprintln(s.Stderr, "")

		if err = upload.reload(); err != nil {
			return err
		}
	}

	return nil
}

func initializationParametersFromMap(params map[string]string) []api.InitializationParameter {
	initializationParameters := make([]api.InitializationParameter, 0, len(params))
	for key, value := range params {
		initializationParameters = append(initializationParameters, api.InitializationParameter{
			Key:   key,
			Value: value,
		})
	}
	return initializationParameters
}

func (s Service) callInitiateRun(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
	initiateStart := time.Now()
	runResult, err := s.APIClient.InitiateRun(cfg)

	s.recordTelemetry("run.initiate", map[string]any{
		"has_targets":     len(cfg.TargetedTaskKeys) > 0,
		"has_init_params": len(cfg.InitializationParameters) > 0,
		"duration_ms":     time.Since(initiateStart).Milliseconds(),
		"success":         err == nil,
	})

	if err != nil {
		return nil, errors.Wrap(err, "Failed to initiate run")
	}

	return runResult, nil
}

// InitiateRun will connect to the Cloud API and start a new run in Mint.
func (s Service) InitiateRun(cfg InitiateRunConfig) (*api.InitiateRunResult, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	paths, cleanupPaths, err := resolveRunDefinitionPaths(cfg.RwxDirectory, cfg.MintFilePath)
	if err != nil {
		return nil, err
	}
	defer cleanupPaths()

	gitMetadata := s.runGitMetadata()
	patchFile := git.PatchFile{}
	patchDir := filepath.Join(paths.rwxDirectoryPath, ".patches")
	defer os.RemoveAll(patchDir)

	patchable := cfg.Patchable && gitMetadata.available
	if _, ok := os.LookupEnv("RWX_DISABLE_GIT_PATCH"); ok {
		patchable = false
	}

	if overridesGitParams, err := s.cliInitParamsOverrideGitParams(paths.relativeRunDefinitionPath, cfg.InitParameters); err != nil {
		return nil, err
	} else if overridesGitParams {
		patchable = false
	}

	if patchable {
		patchPathspec := []string{".", ":!" + paths.relativeRunDefinitionPath}
		if generatedPatch, patchErr := s.GitClient.GeneratePatchFile(patchDir, patchPathspec); patchErr != nil {
			gitMetadata.errorMessage = patchErr.Error()
			fmt.Fprintf(s.Stderr, "Warning: failed to generate patch: %s\n\n", gitMetadata.errorMessage)
		} else {
			patchFile = generatedPatch
		}
	}

	upload, err := loadRunDefinitionUpload(paths, cfg.RwxDirectory)
	if err != nil {
		return nil, err
	}

	if patchFile.Written {
		fmt.Fprintf(s.Stderr, "Included a git patch for uncommitted changes\n")
	}
	if patchFile.UntrackedFiles.Count == 1 {
		fmt.Fprintf(s.Stderr, "The patch did not include the following untracked file. Add it with git add to use it in the run:\n")
		fmt.Fprintf(s.Stderr, "  %s\n", patchFile.UntrackedFiles.Files[0])
	} else if patchFile.UntrackedFiles.Count > 1 {
		fmt.Fprintf(s.Stderr, "The patch did not include the following untracked files. Add them with git add to use them in the run:\n")
		limit := patchFile.UntrackedFiles.Count
		if limit > 5 {
			limit = 5
		}
		for _, file := range patchFile.UntrackedFiles.Files[:limit] {
			fmt.Fprintf(s.Stderr, "  %s\n", file)
		}
		if patchFile.UntrackedFiles.Count > 5 {
			fmt.Fprintf(s.Stderr, "  and %d more\n", patchFile.UntrackedFiles.Count-5)
		}
	}
	if patchFile.Written || patchFile.UntrackedFiles.Count > 0 {
		fmt.Fprintln(s.Stderr, "")
	}

	if err := s.configureRunDefinitionUpload(upload); err != nil {
		return nil, err
	}

	upload.appendRunDefinitionOutsideRwxDir()

	return s.callInitiateRun(api.InitiateRunConfig{
		InitializationParameters: initializationParametersFromMap(cfg.InitParameters),
		TaskDefinitions:          upload.runDefinition,
		RwxDirectory:             upload.rwxDirectory,
		TargetedTaskKeys:         cfg.TargetedTasks,
		Title:                    cfg.Title,
		UseCache:                 !cfg.NoCache,
		CliState:                 cfg.CliState,
		Git:                      gitMetadata.apiGitMetadata(),
		Patch:                    gitMetadata.apiPatchMetadata(patchFile),
	})
}
