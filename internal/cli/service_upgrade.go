package cli

import (
	"bufio"
	"debug/elf"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	semver "github.com/Masterminds/semver/v3"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/rwx-cloud/rwx/internal/releases"
	"github.com/rwx-cloud/rwx/internal/versions"
)

const homebrewUpgradeCommand = "brew upgrade rwx-cloud/tap/rwx"

func upgradeCommand(homebrew bool) string {
	if homebrew {
		return homebrewUpgradeCommand
	}
	return "rwx upgrade"
}

// Overridden in tests.
var (
	executablePath    = os.Executable
	cliCurrentVersion = versions.GetCliCurrentVersion
	runtimeGOOS       = runtime.GOOS
	runtimeGOARCH     = runtime.GOARCH
	sudoAvailable     = func() bool {
		_, err := exec.LookPath("sudo")
		return err == nil
	}
	runSudo = func(args ...string) error {
		cmd := exec.Command("sudo", args...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		return cmd.Run()
	}
)

type UpgradeConfig struct {
	Version string
	Check   bool
	Force   bool
	Json    bool
}

type UpgradeResult struct {
	CurrentVersion   string
	TargetVersion    string
	Path             string
	UpgradeAvailable bool
	Upgraded         bool
}

func (s Service) Upgrade(cfg UpgradeConfig) (*UpgradeResult, error) {
	exe, err := executablePath()
	if err != nil {
		return nil, errors.Wrap(err, "unable to determine the location of the rwx executable")
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return nil, errors.Wrap(err, "unable to determine the location of the rwx executable")
	}

	release, err := s.fetchUpgradeRelease(cfg.Version)
	if err != nil {
		return nil, err
	}

	target, err := semver.NewVersion(release.TagName)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to parse release version %q", release.TagName)
	}
	current := cliCurrentVersion()

	// Without an explicit version only move forward; development builds report
	// a version newer than any release, so they are never replaced by accident.
	upToDate := target.Equal(current) || (cfg.Version == "" && current.GreaterThan(target))
	homebrew := versions.IsHomebrewPath(exe)

	result := &UpgradeResult{
		CurrentVersion:   current.String(),
		TargetVersion:    target.String(),
		Path:             exe,
		UpgradeAvailable: !upToDate,
	}

	if cfg.Check {
		if err := s.outputUpgradeResult(cfg, result, homebrew); err != nil {
			return nil, err
		}
		if result.UpgradeAvailable {
			return result, &ExitCodeError{Code: 1}
		}
		return result, nil
	}

	if upToDate && !cfg.Force {
		return result, s.outputUpgradeResult(cfg, result, homebrew)
	}

	if homebrew {
		return nil, errors.New("rwx was installed with Homebrew. To upgrade, run: " + homebrewUpgradeCommand)
	}

	assetName, err := upgradeAssetName(runtimeGOOS, runtimeGOARCH, runtimeGOOS == "linux" && isStaticallyLinked(exe))
	if err != nil {
		return nil, err
	}
	asset, ok := release.Asset(assetName)
	if !ok {
		return nil, errors.Errorf("release %s has no %s binary", release.TagName, assetName)
	}

	if err := s.installUpgrade(cfg, asset, exe, target); err != nil {
		return nil, err
	}

	result.Upgraded = true
	return result, s.outputUpgradeResult(cfg, result, homebrew)
}

func (s Service) fetchUpgradeRelease(version string) (*releases.Release, error) {
	if version == "" {
		release, err := s.ReleasesClient.Latest()
		if err != nil {
			return nil, errors.Wrap(err, "unable to find the latest rwx release")
		}
		return release, nil
	}

	// Releases are also published under aliases like v3, which track the newest
	// 3.x.x release, so only accept a full version that names one exact binary.
	if _, err := semver.StrictNewVersion(strings.TrimPrefix(version, "v")); err != nil {
		return nil, errors.Errorf("%q is not a full rwx version; specify one like 3.1.0", version)
	}

	tag := "v" + strings.TrimPrefix(version, "v")
	release, err := s.ReleasesClient.Tag(tag)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to find rwx release %s", tag)
	}
	return release, nil
}

// installUpgrade downloads the asset next to the executable, checks that it
// runs, and then renames it over the executable so the swap is atomic.
func (s Service) installUpgrade(cfg UpgradeConfig, asset releases.Asset, exe string, target *semver.Version) error {
	dir := filepath.Dir(exe)

	info, err := os.Stat(exe)
	if err != nil {
		return errors.Wrap(err, "unable to read the current rwx binary")
	}
	// Keep the permissions of the binary being replaced, e.g. 0555 or 0775 installs.
	mode := info.Mode().Perm() | 0o100

	tmp, err := os.CreateTemp(dir, ".rwx-upgrade-*")
	if err != nil {
		if os.IsPermission(err) {
			if s.canUpgradeWithSudo(cfg) {
				return s.installUpgradeWithSudo(asset, exe, mode, target)
			}
			return permissionDeniedError(exe)
		}
		return errors.Wrap(err, "unable to create a temporary file for the upgrade")
	}
	defer os.Remove(tmp.Name())

	if err := s.downloadVerifiedBinary(asset, tmp, mode, target); err != nil {
		return err
	}

	// Windows will not overwrite a running executable, but it will rename one.
	old := exe + ".old"
	if runtimeGOOS == "windows" {
		_ = os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			if os.IsPermission(err) {
				return permissionDeniedError(exe)
			}
			return errors.Wrap(err, "unable to move the current rwx binary aside")
		}
	}

	if err := os.Rename(tmp.Name(), exe); err != nil {
		if runtimeGOOS == "windows" {
			_ = os.Rename(old, exe)
		}
		if os.IsPermission(err) {
			return permissionDeniedError(exe)
		}
		return errors.Wrapf(err, "unable to replace %s", exe)
	}

	return nil
}

// downloadVerifiedBinary downloads the asset into tmp, closes it, and checks
// that the new binary runs and reports the target version.
func (s Service) downloadVerifiedBinary(asset releases.Asset, tmp *os.File, mode os.FileMode, target *semver.Version) error {
	fmt.Fprintf(s.Stderr, "Downloading %s...\n", asset.Name)
	err := s.ReleasesClient.Download(asset, tmp)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return errors.Wrap(err, "unable to download the new rwx binary")
	}

	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return errors.Wrap(err, "unable to make the new rwx binary executable")
	}

	output, err := exec.Command(tmp.Name(), "--version").Output()
	if err != nil {
		return errors.Wrap(err, "the downloaded rwx binary failed to run")
	}
	if reported, ok := reportedVersion(output); !ok || !reported.Equal(target) {
		return errors.Errorf("the downloaded rwx binary reported an unexpected version: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

// canUpgradeWithSudo reports whether someone is at a terminal to confirm an
// upgrade with sudo and enter their password.
func (s Service) canUpgradeWithSudo(cfg UpgradeConfig) bool {
	if runtimeGOOS == "windows" || os.Geteuid() == 0 || cfg.Json || !s.StdinIsTTY || !s.StderrIsTTY {
		return false
	}
	return sudoAvailable()
}

// installUpgradeWithSudo downloads and verifies the new binary as the current
// user, and only uses sudo to copy it next to exe and rename it into place.
func (s Service) installUpgradeWithSudo(asset releases.Asset, exe string, mode os.FileMode, target *semver.Version) error {
	dir := filepath.Dir(exe)

	fmt.Fprintf(s.Stderr, "rwx is installed in %s, which requires root. Replace it using sudo? [y/N] ", dir)
	scanner := bufio.NewScanner(s.Stdin)
	if !scanner.Scan() {
		fmt.Fprintln(s.Stderr)
		return permissionDeniedError(exe)
	}
	if answer := strings.ToLower(strings.TrimSpace(scanner.Text())); answer != "y" && answer != "yes" {
		return permissionDeniedError(exe)
	}

	stagingDir, err := os.MkdirTemp("", "rwx-upgrade-*")
	if err != nil {
		return errors.Wrap(err, "unable to create a temporary directory for the upgrade")
	}
	defer os.RemoveAll(stagingDir)

	tmp, err := os.Create(filepath.Join(stagingDir, "rwx"))
	if err != nil {
		return errors.Wrap(err, "unable to create a temporary file for the upgrade")
	}
	if err := s.downloadVerifiedBinary(asset, tmp, mode, target); err != nil {
		return err
	}

	// Copy into the install directory first so the final swap is a rename on the
	// same filesystem, which is atomic and never writes into the running binary.
	staged := filepath.Join(dir, fmt.Sprintf(".rwx-upgrade-%d", os.Getpid()))
	if err := runSudo("install", "-m", fmt.Sprintf("%o", mode), tmp.Name(), staged); err != nil {
		_ = runSudo("-n", "rm", "-f", staged)
		return errors.Wrapf(err, "unable to copy the new rwx binary into %s with sudo", dir)
	}
	if err := runSudo("mv", "-f", staged, exe); err != nil {
		_ = runSudo("-n", "rm", "-f", staged)
		return errors.Wrapf(err, "unable to replace %s with sudo", exe)
	}

	return nil
}

// permissionDeniedError suggests re-running with the full path, since sudo's
// secure_path often leaves out where rwx is installed (e.g. /usr/local/bin on RHEL).
func permissionDeniedError(exe string) error {
	return errors.Errorf("unable to replace %s without elevated permissions. To upgrade, run: sudo %s upgrade", exe, exe)
}

// reportedVersion parses the output of `rwx --version`, e.g. "rwx version v3.1.0".
func reportedVersion(output []byte) (*semver.Version, bool) {
	line, _, _ := strings.Cut(string(output), "\n")
	raw, ok := strings.CutPrefix(strings.TrimSpace(line), "rwx version ")
	if !ok {
		return nil, false
	}

	version, err := semver.NewVersion(raw)
	if err != nil {
		return nil, false
	}
	return version, true
}

func (s Service) outputUpgradeResult(cfg UpgradeConfig, result *UpgradeResult, homebrew bool) error {
	if cfg.Json {
		encoded, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return errors.Wrap(err, "unable to JSON encode the result")
		}

		fmt.Fprintln(s.Stdout, string(encoded))
		return nil
	}

	switch {
	case result.Upgraded:
		fmt.Fprintf(s.Stdout, "Upgraded rwx %s → %s (%s)\n", result.CurrentVersion, result.TargetVersion, result.Path)
	case result.UpgradeAvailable:
		fmt.Fprintf(s.Stdout, "A new release of rwx is available: %s → %s\n", result.CurrentVersion, result.TargetVersion)
		fmt.Fprintln(s.Stdout, "To upgrade, run: "+upgradeCommand(homebrew))
	default:
		fmt.Fprintf(s.Stdout, "rwx %s is already up to date\n", result.CurrentVersion)
	}

	return nil
}

// upgradeAssetName maps a platform to the binary name published on each GitHub
// release, e.g. rwx-linux-x86_64 or rwx-windows-aarch64.exe.
func upgradeAssetName(goos, goarch string, static bool) (string, error) {
	var arch string
	switch goarch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "aarch64"
	default:
		return "", errors.Errorf("rwx does not publish binaries for %s/%s", goos, goarch)
	}

	switch goos {
	case "linux", "darwin", "windows":
	default:
		return "", errors.Errorf("rwx does not publish binaries for %s/%s", goos, goarch)
	}

	name := fmt.Sprintf("rwx-%s-%s", goos, arch)
	// Only linux/amd64 has a separate -static build. The linux/arm64 build is
	// cross-compiled without cgo, so it looks static but is published unsuffixed.
	if static && goos == "linux" && goarch == "amd64" {
		name += "-static"
	}
	if goos == "windows" {
		name += ".exe"
	}
	return name, nil
}

// isStaticallyLinked reports whether the ELF binary at path has no dynamic
// interpreter, which is how the rwx-linux-*-static builds differ.
func isStaticallyLinked(path string) bool {
	f, err := elf.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	for _, prog := range f.Progs {
		if prog.Type == elf.PT_INTERP {
			return false
		}
	}
	return true
}
