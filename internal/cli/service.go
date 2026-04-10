package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
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
		fmt.Fprintln(w, "To upgrade: rwx skill update")
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

type SkillUpdateEntry struct {
	Installation skill.Installation
	OldVersion   string
	NewVersion   string
	Action       string // "updated" or "skipped"
}

type SkillUpdateResult struct {
	Entries []SkillUpdateEntry
}

// SkillUpdate updates outdated agents skill installations by fetching
// the latest SKILL.md from GitHub. Marketplace installations are skipped.
func (s Service) SkillUpdate(symlink string) (*SkillUpdateResult, error) {
	result, err := skill.Detect()
	if err != nil {
		return nil, err
	}

	if !result.AnyFound {
		return &SkillUpdateResult{}, nil
	}

	latestVersionStr := s.fetchLatestSkillVersion()
	if latestVersionStr == "" {
		return nil, errors.New("unable to determine the latest skill version")
	}
	latestVersion, err := semver.NewVersion(latestVersionStr)
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse latest skill version")
	}

	var entries []SkillUpdateEntry
	var needsFetch bool

	for _, inst := range result.Installations {
		if !skill.IsDetected(inst) {
			continue
		}

		outdated := false
		if inst.Version == "" {
			outdated = true
		} else {
			v, err := semver.NewVersion(inst.Version)
			if err == nil && latestVersion.GreaterThan(v) {
				outdated = true
			}
		}

		if !outdated {
			continue
		}

		if inst.Source == "marketplace" {
			entries = append(entries, SkillUpdateEntry{
				Installation: inst,
				OldVersion:   inst.Version,
				Action:       "skipped",
			})
			continue
		}

		needsFetch = true
		entries = append(entries, SkillUpdateEntry{
			Installation: inst,
			OldVersion:   inst.Version,
			NewVersion:   latestVersionStr,
			Action:       "updated",
		})
	}

	if needsFetch {
		content, err := skill.FetchSkillContent()
		if err != nil {
			return nil, err
		}

		for _, entry := range entries {
			if entry.Action != "updated" {
				continue
			}
			if err := os.WriteFile(entry.Installation.Path, []byte(content), 0o644); err != nil {
				return nil, errors.Wrap(err, "unable to write skill file")
			}
		}
	}

	if needsFetch && symlink == "claude" {
		cwd, err := os.Getwd()
		if err == nil {
			s.ensureClaudeSkillsSymlink(cwd)
		}
	}

	return &SkillUpdateResult{Entries: entries}, nil
}

type SkillInstallResult struct {
	Path string
}

// SkillInstall installs the RWX agent skill at the project level. The
// install location and symlink behavior are inferred from the project:
//   - Neither .agents nor .claude exists: install to .agents, prompt for .claude symlink
//   - Both exist: install to .agents, auto-symlink to .claude
//   - Only .claude exists: install directly to .claude/skills/rwx/SKILL.md
//   - Only .agents exists: install to .agents, no symlink
//
// The --symlink claude flag forces symlink creation (useful in non-TTY).
func (s Service) SkillInstall(yes bool, symlink string) (*SkillInstallResult, error) {
	detected, err := skill.Detect()
	if err != nil {
		return nil, err
	}

	for _, inst := range detected.Installations {
		if skill.IsDetected(inst) && inst.Source == "agents" {
			fmt.Fprintf(s.Stderr, "An existing %s installation was found at %s\n", inst.Scope, inst.Path)
			if err := s.confirmDestruction("Install at the project level anyway?", yes); err != nil {
				return nil, err
			}
			break
		}
	}

	content, err := skill.FetchSkillContent()
	if err != nil {
		return nil, err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, errors.Wrap(err, "unable to determine working directory")
	}

	hasAgents := dirExists(filepath.Join(cwd, ".agents"))
	hasClaude := dirExists(filepath.Join(cwd, ".claude"))

	var skillPath string
	switch {
	case hasClaude && !hasAgents:
		// Only .claude exists — install directly there.
		skillDir := filepath.Join(cwd, ".claude", "skills", "rwx")
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			return nil, errors.Wrap(err, "unable to create skill directory")
		}
		skillPath = filepath.Join(skillDir, "SKILL.md")

	default:
		// Install to .agents (the canonical location).
		skillDir := filepath.Join(cwd, ".agents", "skills", "rwx")
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			return nil, errors.Wrap(err, "unable to create skill directory")
		}
		skillPath = filepath.Join(skillDir, "SKILL.md")

		shouldSymlink := symlink == "claude"
		if !shouldSymlink && hasAgents && hasClaude {
			// Both directories exist — auto-symlink without prompting.
			shouldSymlink = true
		}
		if !shouldSymlink && !hasAgents && !hasClaude {
			// Neither directory exists — prompt in TTY, or honor the flag.
			shouldSymlink = s.promptForClaudeSymlink()
		}
		if shouldSymlink {
			s.ensureClaudeSkillsSymlink(cwd)
		}
	}

	if err := os.WriteFile(skillPath, []byte(content), 0o644); err != nil {
		return nil, errors.Wrap(err, "unable to write skill file")
	}

	return &SkillInstallResult{Path: skillPath}, nil
}

// promptForClaudeSymlink asks the user whether to create a Claude Code symlink.
// Returns true if the user confirms. Only prompts in TTY mode.
func (s Service) promptForClaudeSymlink() bool {
	if !s.StderrIsTTY {
		return false
	}

	fmt.Fprintf(s.Stderr, "\nSymlink .claude/skills? This is required for Claude Code to discover the skill in .agents/skills/rwx/SKILL.md, so it's recommended if anyone on this project uses Claude Code.\n")
	fmt.Fprintf(s.Stderr, "Create symlink? [y/N]: ")
	scanner := bufio.NewScanner(s.Stdin)
	if !scanner.Scan() {
		return false
	}

	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return answer == "y" || answer == "yes"
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// ensureClaudeSkillsSymlink ensures Claude Code can discover the installed
// skill. If .claude/skills doesn't exist, it's created as a symlink to
// .agents/skills. If it already exists as a directory, a symlink is created
// at .claude/skills/rwx pointing to .agents/skills/rwx.
func (s Service) ensureClaudeSkillsSymlink(projectDir string) {
	claudeSkills := filepath.Join(projectDir, ".claude", "skills")
	agentsSkills := filepath.Join(projectDir, ".agents", "skills")

	info, err := os.Lstat(claudeSkills)
	if err != nil {
		// .claude/skills doesn't exist — create it as a symlink to .agents/skills.
		_ = os.MkdirAll(filepath.Join(projectDir, ".claude"), 0o755)
		_ = os.Symlink(agentsSkills, claudeSkills)
		return
	}

	// If it's already a symlink, check whether it points to .agents/skills.
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(claudeSkills)
		if err == nil && target == agentsSkills {
			return
		}
	}

	// .claude/skills exists as a real directory (or a symlink to somewhere else).
	// Add a per-skill symlink so we don't clobber existing content.
	claudeRwx := filepath.Join(claudeSkills, "rwx")
	agentsRwx := filepath.Join(agentsSkills, "rwx")
	if _, err := os.Lstat(claudeRwx); err != nil {
		_ = os.Symlink(agentsRwx, claudeRwx)
	}
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
	CommitSha      string
}

// ResolveRunIDFromGitContext resolves the latest run ID by looking up the
// current git branch and repository name via the API. Fields set on the
// optional config override values that would otherwise be inferred from git.
func (s Service) ResolveRunIDFromGitContext(overrides ...ResolveRunIDConfig) (string, error) {
	var cfg ResolveRunIDConfig
	if len(overrides) > 0 {
		cfg = overrides[0]
	}

	branchName := cfg.BranchName
	if branchName == "" && cfg.CommitSha == "" {
		branchName = s.GitClient.GetBranch()
	}
	repositoryName := git.RepoNameFromOriginUrl(s.GitClient.GetOriginUrl())
	if cfg.RepositoryName != "" {
		repositoryName = cfg.RepositoryName
	}

	if repositoryName == "" || (branchName == "" && cfg.CommitSha == "") {
		return "", errors.New("unable to determine the current branch and repository from git; please provide a run ID")
	}

	result, err := s.APIClient.RunStatus(api.RunStatusConfig{
		BranchName:     branchName,
		RepositoryName: repositoryName,
		DefinitionPath: cfg.DefinitionPath,
		CommitSha:      cfg.CommitSha,
	})
	notFoundMsg := fmt.Sprintf("no run found for %s repository", repositoryName)
	if branchName != "" {
		notFoundMsg += fmt.Sprintf(" on branch %s", branchName)
	}
	if cfg.DefinitionPath != "" {
		notFoundMsg += fmt.Sprintf(" with definition %s", cfg.DefinitionPath)
	}
	if cfg.CommitSha != "" {
		notFoundMsg += fmt.Sprintf(" at commit %s", cfg.CommitSha)
	}

	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			return "", fmt.Errorf("%s", notFoundMsg)
		}
		if errors.Is(err, errors.ErrAmbiguousDefinitionPath) {
			return "", err
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
