package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	semver "github.com/Masterminds/semver/v3"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/rwx-cloud/rwx/internal/git"
	"github.com/rwx-cloud/rwx/internal/skill"
	"github.com/rwx-cloud/rwx/internal/versions"
)

const DefaultArch = "x86_64"

var HandledError = errors.New("handled error")

// ExitCodeError signals that the process should exit with a specific code.
// Commands return this instead of calling os.Exit directly so that main()
// can flush telemetry before exiting.
type ExitCodeError struct {
	Code int
}

func (e *ExitCodeError) Error() string {
	return fmt.Sprintf("exit code %d", e.Code)
}

func (e *ExitCodeError) Is(target error) bool {
	_, ok := target.(*ExitCodeError)
	return ok
}

var hasOutputVersionMessage atomic.Bool
var hasOutputSkillMessage atomic.Bool

// Service holds the main business logic of the CLI.
type Service struct {
	Config
}

func NewService(cfg Config) (Service, error) {
	if err := cfg.Validate(); err != nil {
		return Service{}, errors.Wrap(err, "validation failed")
	}

	svc := Service{cfg}
	svc.outputLatestVersionMessage()
	svc.outputOutdatedSkillMessage()
	return svc, nil
}

func (s Service) outputLatestVersionMessage() {
	versions.LoadLatestVersionFromFile(s.VersionsBackend)
	versions.LoadLatestSkillVersionFromFile(s.SkillVersionsBackend)

	if !versions.NewVersionAvailable() {
		return
	}

	if !hasOutputVersionMessage.CompareAndSwap(false, true) {
		return
	}

	showLatestVersion := os.Getenv("MINT_HIDE_LATEST_VERSION") == "" && os.Getenv("RWX_HIDE_LATEST_VERSION") == ""

	if !showLatestVersion {
		return
	}

	w := s.Stderr
	fmt.Fprintf(w, "A new release of rwx is available: %s → %s\n", versions.GetCliCurrentVersion(), versions.GetCliLatestVersion())

	if versions.InstalledWithHomebrew() {
		fmt.Fprintln(w, "To upgrade, run: brew upgrade rwx-cloud/tap/rwx")
	}

	fmt.Fprintln(w)
}

func (s Service) outputOutdatedSkillMessage() {
	if os.Getenv("RWX_HIDE_SKILL_HINT") != "" {
		return
	}

	latestVersion := versions.GetSkillLatestVersion()
	if latestVersion.Equal(versions.EmptyVersion) {
		return
	}

	result, err := skill.Detect()
	if err != nil || !result.AnyFound {
		return
	}

	// Collect which sources are outdated and track the highest outdated version.
	var highestOutdated *semver.Version
	outdatedSources := make(map[string]bool)
	for _, inst := range result.Installations {
		if !skill.IsDetected(inst) {
			continue
		}
		if inst.Version == "" {
			// Installations with no version in frontmatter are always considered outdated.
			outdatedSources[inst.Source] = true
			continue
		}
		v, err := semver.NewVersion(inst.Version)
		if err != nil {
			continue
		}
		if latestVersion.GreaterThan(v) {
			outdatedSources[inst.Source] = true
			if highestOutdated == nil || v.GreaterThan(highestOutdated) {
				highestOutdated = v
			}
		}
	}

	if len(outdatedSources) == 0 {
		return
	}

	if !hasOutputSkillMessage.CompareAndSwap(false, true) {
		return
	}

	w := s.Stderr
	if highestOutdated != nil {
		fmt.Fprintf(w, "\nA new version of the RWX agent skill is available: v%s → v%s\n", highestOutdated, latestVersion)
	} else {
		fmt.Fprintf(w, "\nA new version of the RWX agent skill is available: v%s\n", latestVersion)
	}
	if outdatedSources["agents"] {
		fmt.Fprintln(w, "To upgrade: npx skills update rwx")
	}
	if outdatedSources["marketplace"] {
		fmt.Fprintln(w, "To upgrade the Claude Code marketplace: claude plugin marketplace update rwx && claude plugin update rwx@rwx")
	}
	fmt.Fprintln(w)
}

type SkillStatusResult struct {
	Installations []skill.Installation
	AnyFound      bool
	LatestVersion string
}

// SkillStatus detects installed skills and fetches the latest available version.
// The API call is skipped if the file cache is fresh or if the fetch fails.
func (s Service) SkillStatus() (*SkillStatusResult, error) {
	result, err := skill.Detect()
	if err != nil {
		return nil, err
	}

	latestVersion := s.fetchLatestSkillVersion()

	return &SkillStatusResult{
		Installations: result.Installations,
		AnyFound:      result.AnyFound,
		LatestVersion: latestVersion,
	}, nil
}

// fetchLatestSkillVersion returns the latest skill version, fetching from the
// API if the file cache is stale. Returns empty string if unavailable.
func (s Service) fetchLatestSkillVersion() string {
	// Check whether the file cache is still fresh.
	if s.SkillVersionsBackend != nil {
		if modTime, err := s.SkillVersionsBackend.ModTime(); err == nil {
			if time.Since(modTime) < versions.SkillVersionCacheTTL {
				versions.LoadLatestSkillVersionFromFile(s.SkillVersionsBackend)
				v := versions.GetSkillLatestVersion()
				if !v.Equal(versions.EmptyVersion) {
					return v.String()
				}
			}
		}
	}

	// Cache is stale or empty — call the API.
	versionStr, err := s.APIClient.GetSkillLatestVersion()
	if err != nil || versionStr == "" {
		return ""
	}

	_ = versions.SetSkillLatestVersion(versionStr)
	versions.SaveLatestSkillVersionToFile(s.SkillVersionsBackend)
	return versionStr
}

// recordTelemetry enqueues a telemetry event if a collector is configured.
func (s Service) recordTelemetry(event string, props map[string]any) {
	if s.TelemetryCollector == nil {
		return
	}
	s.TelemetryCollector.Record(event, props)
}

// confirmDestruction prompts the user to confirm a destructive action.
// If yes is true, confirmation is skipped. In non-TTY environments without
// yes, an error is returned instructing the user to pass --yes.
func (s Service) confirmDestruction(prompt string, yes bool) error {
	if yes {
		return nil
	}

	if !s.StderrIsTTY {
		return errors.New("use --yes to confirm in non-interactive environments")
	}

	fmt.Fprintf(s.Stderr, "%s [y/N]: ", prompt)
	scanner := bufio.NewScanner(s.Stdin)
	if !scanner.Scan() {
		return errors.New("no input provided")
	}

	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	if answer != "y" && answer != "yes" {
		return errors.New("aborted")
	}

	return nil
}

type ResolveRunIDConfig struct {
	BranchName     string
	RepositoryName string
	DefinitionPath string
}

// ResolveRunIDFromGitContext resolves the latest run ID by looking up the
// current git branch and repository name via the API. Fields set on the
// optional config override values that would otherwise be inferred from git.
func (s Service) ResolveRunIDFromGitContext(overrides ...ResolveRunIDConfig) (string, error) {
	var cfg ResolveRunIDConfig
	if len(overrides) > 0 {
		cfg = overrides[0]
	}

	branchName := s.GitClient.GetBranch()
	if cfg.BranchName != "" {
		branchName = cfg.BranchName
	}
	repositoryName := git.RepoNameFromOriginUrl(s.GitClient.GetOriginUrl())
	if cfg.RepositoryName != "" {
		repositoryName = cfg.RepositoryName
	}

	if branchName == "" || repositoryName == "" {
		return "", errors.New("unable to determine the current branch and repository from git; please provide a run ID")
	}

	result, err := s.APIClient.RunStatus(api.RunStatusConfig{
		BranchName:     branchName,
		RepositoryName: repositoryName,
		DefinitionPath: cfg.DefinitionPath,
	})
	notFoundMsg := fmt.Sprintf("no run found for %s repository on branch %s", repositoryName, branchName)
	if cfg.DefinitionPath != "" {
		notFoundMsg += fmt.Sprintf(" with definition %s", cfg.DefinitionPath)
	}

	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			return "", fmt.Errorf("%s", notFoundMsg)
		}
		return "", errors.Wrap(err, "unable to resolve run from git context")
	}

	if result.RunID == "" {
		return "", fmt.Errorf("%s", notFoundMsg)
	}

	return result.RunID, nil
}

func Map[T any, R any](input []T, transformer func(T) R) []R {
	result := make([]R, len(input))
	for i, item := range input {
		result[i] = transformer(item)
	}
	return result
}

func tryGetSliceAtIndex[S ~[]E, E any](s S, index int, defaultValue E) E {
	if len(s) <= index {
		return defaultValue
	}
	return s[index]
}
