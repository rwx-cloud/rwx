package cli_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/mocks"
	"github.com/rwx-cloud/rwx/internal/telemetry"
	"github.com/rwx-cloud/rwx/internal/versions"
	"github.com/stretchr/testify/require"
)

// setupSkillTest prepares an isolated environment for testing the skill nag.
// Unlike setupTest, it sets HOME to the temp dir (isolating from real global
// skill installations) and does not call NewService, allowing the caller to
// configure version state before the nag check runs.
func setupSkillTest(t *testing.T) *testSetup {
	t.Helper()
	setup := &testSetup{}

	var err error
	setup.tmp, err = os.MkdirTemp(os.TempDir(), "cli-service")
	require.NoError(t, err)
	setup.tmp, err = filepath.EvalSymlinks(setup.tmp)
	require.NoError(t, err)

	setup.originalWd, err = os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(setup.tmp))
	require.NoError(t, os.Mkdir(filepath.Join(setup.tmp, ".rwx"), 0o755))

	// Isolate from real global skill installations.
	t.Setenv("HOME", setup.tmp)

	setup.mockAPI = new(mocks.API)
	setup.mockSSH = new(mocks.SSH)
	setup.mockGit = &mocks.Git{
		MockIsInstalled:      true,
		MockIsInsideWorkTree: true,
	}
	setup.mockDocker = new(mocks.DockerClient)
	setup.collector = telemetry.NewCollector()
	setup.mockStdin = &bytes.Buffer{}
	setup.mockStdout = &strings.Builder{}
	setup.mockStderr = &strings.Builder{}

	setup.config = cli.Config{
		APIClient:          setup.mockAPI,
		SSHClient:          setup.mockSSH,
		GitClient:          setup.mockGit,
		DockerCLI:          setup.mockDocker,
		TelemetryCollector: setup.collector,
		Stdin:              setup.mockStdin,
		Stdout:             setup.mockStdout,
		StdoutIsTTY:        false,
		Stderr:             setup.mockStderr,
		StderrIsTTY:        false,
	}

	t.Cleanup(func() {
		require.NoError(t, os.Chdir(setup.originalWd))
		require.NoError(t, os.RemoveAll(setup.tmp))
	})

	return setup
}

func seedSkillFile(t *testing.T, dir, version string) {
	t.Helper()
	skillDir := filepath.Join(dir, ".agents", "skills", "rwx")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	content := "---\nmetadata:\n  version: " + version + "\n---\nSkill content\n"
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644))
}

func seedMarketplaceSkillFile(t *testing.T, homeDir, version string) {
	t.Helper()
	skillDir := filepath.Join(homeDir, ".claude", "plugins", "marketplaces", "rwx", "plugins", "rwx", "skills", "rwx")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	content := "---\nmetadata:\n  version: " + version + "\n---\nSkill content\n"
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644))
}

func TestOutputOutdatedSkillMessage(t *testing.T) {
	// The nag is guarded by a package-level atomic bool, so only one subtest
	// can trigger the actual print per process. Negative cases (which return
	// before the atomic) are ordered first; the positive case runs last.

	t.Run("no skill installed", func(t *testing.T) {
		s := setupSkillTest(t)
		_ = versions.SetSkillLatestVersion("2.0.0")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)
		require.Empty(t, s.mockStderr.String())
	})

	t.Run("installed version equals latest", func(t *testing.T) {
		s := setupSkillTest(t)
		seedSkillFile(t, s.tmp, "2.0.0")
		_ = versions.SetSkillLatestVersion("2.0.0")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)
		require.Empty(t, s.mockStderr.String())
	})

	t.Run("latest version unknown", func(t *testing.T) {
		s := setupSkillTest(t)
		seedSkillFile(t, s.tmp, "1.0.0")
		_ = versions.SetSkillLatestVersion("0.0.0")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)
		require.Empty(t, s.mockStderr.String())
	})

	t.Run("suppressed by env var", func(t *testing.T) {
		s := setupSkillTest(t)
		seedSkillFile(t, s.tmp, "1.0.0")
		_ = versions.SetSkillLatestVersion("2.0.0")
		t.Setenv("RWX_HIDE_SKILL_HINT", "1")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)
		require.Empty(t, s.mockStderr.String())
	})

	t.Run("prints upgrade instructions including installations with no version", func(t *testing.T) {
		s := setupSkillTest(t)
		seedSkillFile(t, s.tmp, "")
		seedMarketplaceSkillFile(t, s.tmp, "1.1.0")
		_ = versions.SetSkillLatestVersion("2.0.0")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)

		output := s.mockStderr.String()
		require.Contains(t, output, "A new version of the RWX agent skill is available: v1.1.0 → v2.0.0")
		require.Contains(t, output, "To upgrade: npx skills update rwx")
		require.Contains(t, output, "To upgrade the Claude Code marketplace: claude plugin marketplace update rwx && claude plugin update rwx@rwx")
	})
}

func TestSkillStatus(t *testing.T) {
	t.Run("fetches latest version from API when cache is empty", func(t *testing.T) {
		s := setupSkillTest(t)
		seedSkillFile(t, s.tmp, "1.0.0")

		s.mockAPI.MockGetSkillLatestVersion = func() (string, error) {
			return "2.0.0", nil
		}

		// Suppress the nag from NewService so it does not interfere.
		t.Setenv("RWX_HIDE_SKILL_HINT", "1")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)

		result, err := s.service.SkillStatus()
		require.NoError(t, err)
		require.True(t, result.AnyFound)
		require.Equal(t, "2.0.0", result.LatestVersion)
	})

	t.Run("uses cached version when fresh", func(t *testing.T) {
		s := setupSkillTest(t)
		seedSkillFile(t, s.tmp, "1.0.0")

		backend := versions.NewMemoryBackend()
		_ = backend.Set("3.0.0")
		s.config.SkillVersionsBackend = backend

		// API should not be called.
		s.mockAPI.MockGetSkillLatestVersion = func() (string, error) {
			t.Fatal("API should not be called when cache is fresh")
			return "", nil
		}

		t.Setenv("RWX_HIDE_SKILL_HINT", "1")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)

		result, err := s.service.SkillStatus()
		require.NoError(t, err)
		require.Equal(t, "3.0.0", result.LatestVersion)
	})

	t.Run("fetches from API when cache is stale", func(t *testing.T) {
		s := setupSkillTest(t)
		seedSkillFile(t, s.tmp, "1.0.0")

		backend := versions.NewMemoryBackend()
		_ = backend.Set("1.5.0")
		backend.SetModTime(time.Now().Add(-3 * time.Hour))
		s.config.SkillVersionsBackend = backend

		s.mockAPI.MockGetSkillLatestVersion = func() (string, error) {
			return "2.0.0", nil
		}

		t.Setenv("RWX_HIDE_SKILL_HINT", "1")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)

		result, err := s.service.SkillStatus()
		require.NoError(t, err)
		require.Equal(t, "2.0.0", result.LatestVersion)
	})

	t.Run("returns empty latest version when API fails", func(t *testing.T) {
		s := setupSkillTest(t)
		seedSkillFile(t, s.tmp, "1.0.0")

		s.mockAPI.MockGetSkillLatestVersion = func() (string, error) {
			return "", fmt.Errorf("network error")
		}

		t.Setenv("RWX_HIDE_SKILL_HINT", "1")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)

		result, err := s.service.SkillStatus()
		require.NoError(t, err)
		require.True(t, result.AnyFound)
		require.Empty(t, result.LatestVersion)
	})

	t.Run("no skill installed", func(t *testing.T) {
		s := setupSkillTest(t)

		s.mockAPI.MockGetSkillLatestVersion = func() (string, error) {
			return "2.0.0", nil
		}

		t.Setenv("RWX_HIDE_SKILL_HINT", "1")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)

		result, err := s.service.SkillStatus()
		require.NoError(t, err)
		require.False(t, result.AnyFound)
	})
}
