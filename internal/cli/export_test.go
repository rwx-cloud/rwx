package cli

import (
	"time"

	semver "github.com/Masterminds/semver/v3"
)

// SetArtifactReadyMaxWaitForTest overrides the artifact polling deadline so tests don't have to
// wait the full production duration. It returns a function that restores the previous value.
func SetArtifactReadyMaxWaitForTest(d time.Duration) (restore func()) {
	prev := artifactReadyMaxWait
	artifactReadyMaxWait = d
	return func() { artifactReadyMaxWait = prev }
}

// SetUpgradeEnvironmentForTest overrides the executable path, current version and
// platform used by Upgrade. It returns a function that restores the previous values.
func SetUpgradeEnvironmentForTest(exe string, current string, goos string, goarch string) (restore func()) {
	prevExecutablePath, prevCurrentVersion := executablePath, cliCurrentVersion
	prevGOOS, prevGOARCH := runtimeGOOS, runtimeGOARCH

	executablePath = func() (string, error) { return exe, nil }
	cliCurrentVersion = func() *semver.Version { return semver.MustParse(current) }
	runtimeGOOS, runtimeGOARCH = goos, goarch

	return func() {
		executablePath, cliCurrentVersion = prevExecutablePath, prevCurrentVersion
		runtimeGOOS, runtimeGOARCH = prevGOOS, prevGOARCH
	}
}

// SetSudoForTest overrides whether sudo is available and how it is run during
// Upgrade. It returns a function that restores the previous values.
func SetSudoForTest(available bool, run func(args ...string) error) (restore func()) {
	prevAvailable, prevRun := sudoAvailable, runSudo

	sudoAvailable = func() bool { return available }
	runSudo = run

	return func() {
		sudoAvailable, runSudo = prevAvailable, prevRun
	}
}

var UpgradeAssetName = upgradeAssetName

// ResetLatestVersionMessageForTest allows the latest version message to be output again.
func ResetLatestVersionMessageForTest() {
	hasOutputVersionMessage.Store(false)
}
