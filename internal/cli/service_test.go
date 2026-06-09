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
		require.Contains(t, output, "To upgrade: rwx skill update")
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

func TestSkillUpdate(t *testing.T) {
	t.Run("no installations returns empty entries", func(t *testing.T) {
		s := setupSkillTest(t)

		backend := versions.NewMemoryBackend()
		_ = backend.Set("2.0.0")
		s.config.SkillVersionsBackend = backend

		t.Setenv("RWX_HIDE_SKILL_HINT", "1")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)

		result, err := s.service.SkillUpdate("")
		require.NoError(t, err)
		require.Empty(t, result.Entries)
	})

	t.Run("all up to date returns empty entries", func(t *testing.T) {
		s := setupSkillTest(t)
		seedSkillFile(t, s.tmp, "2.0.0")

		backend := versions.NewMemoryBackend()
		_ = backend.Set("2.0.0")
		s.config.SkillVersionsBackend = backend

		t.Setenv("RWX_HIDE_SKILL_HINT", "1")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)

		result, err := s.service.SkillUpdate("")
		require.NoError(t, err)
		require.Empty(t, result.Entries)
	})

	t.Run("updates outdated installation with content from API", func(t *testing.T) {
		s := setupSkillTest(t)
		seedSkillFile(t, s.tmp, "1.0.0")

		backend := versions.NewMemoryBackend()
		_ = backend.Set("2.0.0")
		s.config.SkillVersionsBackend = backend

		newContent := "---\nmetadata:\n  version: 2.0.0\n---\nUpdated skill content\n"
		s.mockAPI.MockGetSkillContent = func() (string, error) {
			return newContent, nil
		}

		t.Setenv("RWX_HIDE_SKILL_HINT", "1")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)

		result, err := s.service.SkillUpdate("")
		require.NoError(t, err)
		require.Len(t, result.Entries, 1)
		require.Equal(t, "updated", result.Entries[0].Action)
		require.Equal(t, "1.0.0", result.Entries[0].OldVersion)
		require.Equal(t, "2.0.0", result.Entries[0].NewVersion)

		written, err := os.ReadFile(result.Entries[0].Installation.Path)
		require.NoError(t, err)
		require.Equal(t, newContent, string(written))
	})

	t.Run("skips marketplace installations", func(t *testing.T) {
		s := setupSkillTest(t)
		seedMarketplaceSkillFile(t, s.tmp, "1.0.0")

		backend := versions.NewMemoryBackend()
		_ = backend.Set("2.0.0")
		s.config.SkillVersionsBackend = backend

		s.mockAPI.MockGetSkillLatestVersion = func() (string, error) {
			return "2.0.0", nil
		}

		t.Setenv("RWX_HIDE_SKILL_HINT", "1")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)

		result, err := s.service.SkillUpdate("")
		require.NoError(t, err)
		require.Len(t, result.Entries, 1)
		require.Equal(t, "skipped", result.Entries[0].Action)
		require.Equal(t, "marketplace", result.Entries[0].Installation.Source)
	})

	t.Run("unparseable latest version returns empty entries", func(t *testing.T) {
		s := setupSkillTest(t)
		seedSkillFile(t, s.tmp, "1.0.0")

		s.mockAPI.MockGetSkillLatestVersion = func() (string, error) {
			return "dev", nil
		}

		t.Setenv("RWX_HIDE_SKILL_HINT", "1")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)

		result, err := s.service.SkillUpdate("")
		require.NoError(t, err)
		require.Empty(t, result.Entries)
	})
}

func TestSkillInstall(t *testing.T) {
	t.Run("writes SKILL.md to project directory", func(t *testing.T) {
		s := setupSkillTest(t)

		skillContent := "---\nmetadata:\n  version: 2.0.0\n---\nSkill content\n"
		s.mockAPI.MockGetSkillContent = func() (string, error) {
			return skillContent, nil
		}

		t.Setenv("RWX_HIDE_SKILL_HINT", "1")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)

		result, err := s.service.SkillInstall(true, "")
		require.NoError(t, err)

		expectedPath := filepath.Join(s.tmp, ".agents", "skills", "rwx", "SKILL.md")
		require.Equal(t, expectedPath, result.Path)

		written, err := os.ReadFile(expectedPath)
		require.NoError(t, err)
		require.Equal(t, skillContent, string(written))
	})

	t.Run("--yes installs to repo and prints user-level handoff", func(t *testing.T) {
		s := setupSkillTest(t)

		s.mockAPI.MockGetSkillContent = func() (string, error) {
			return "---\nmetadata:\n  version: 2.0.0\n---\nSkill content\n", nil
		}

		t.Setenv("RWX_HIDE_SKILL_HINT", "1")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)

		result, err := s.service.SkillInstall(true, "")
		require.NoError(t, err)
		require.Equal(t, filepath.Join(s.tmp, ".agents", "skills", "rwx", "SKILL.md"), result.Path)

		stderr := s.mockStderr.String()
		require.Contains(t, stderr, "npx skills add rwx-cloud/skills")
		require.Contains(t, stderr, "https://www.rwx.com/docs/ai")
	})

	t.Run("non-TTY without --yes installs to repo without handoff", func(t *testing.T) {
		s := setupSkillTest(t)

		s.mockAPI.MockGetSkillContent = func() (string, error) {
			return "---\nmetadata:\n  version: 2.0.0\n---\nSkill content\n", nil
		}

		t.Setenv("RWX_HIDE_SKILL_HINT", "1")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)

		result, err := s.service.SkillInstall(false, "")
		require.NoError(t, err)
		require.Equal(t, filepath.Join(s.tmp, ".agents", "skills", "rwx", "SKILL.md"), result.Path)

		require.NotContains(t, s.mockStderr.String(), "npx skills")
	})

	t.Run("interactive picks repo and installs", func(t *testing.T) {
		s := setupSkillTest(t)
		s.config.StderrIsTTY = true
		s.mockStdin = bytes.NewBufferString("1\n")
		s.config.Stdin = s.mockStdin

		s.mockAPI.MockGetSkillContent = func() (string, error) {
			return "---\nmetadata:\n  version: 2.0.0\n---\nSkill content\n", nil
		}

		t.Setenv("RWX_HIDE_SKILL_HINT", "1")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)

		result, err := s.service.SkillInstall(false, "")
		require.NoError(t, err)
		require.Equal(t, filepath.Join(s.tmp, ".agents", "skills", "rwx", "SKILL.md"), result.Path)

		stderr := s.mockStderr.String()
		require.Contains(t, stderr, "Where would you like to install the RWX skill?")
		require.NotContains(t, stderr, "npx skills")
	})

	t.Run("interactive default (empty) installs to repo", func(t *testing.T) {
		s := setupSkillTest(t)
		s.config.StderrIsTTY = true
		s.mockStdin = bytes.NewBufferString("\n")
		s.config.Stdin = s.mockStdin

		s.mockAPI.MockGetSkillContent = func() (string, error) {
			return "---\nmetadata:\n  version: 2.0.0\n---\nSkill content\n", nil
		}

		t.Setenv("RWX_HIDE_SKILL_HINT", "1")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)

		result, err := s.service.SkillInstall(false, "")
		require.NoError(t, err)
		require.Equal(t, filepath.Join(s.tmp, ".agents", "skills", "rwx", "SKILL.md"), result.Path)
	})

	t.Run("interactive picks user and prints handoff without installing", func(t *testing.T) {
		s := setupSkillTest(t)
		s.config.StderrIsTTY = true
		s.mockStdin = bytes.NewBufferString("2\n")
		s.config.Stdin = s.mockStdin

		s.mockAPI.MockGetSkillContent = func() (string, error) {
			t.Fatal("API should not be called when user picks user-level install")
			return "", nil
		}

		t.Setenv("RWX_HIDE_SKILL_HINT", "1")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)

		result, err := s.service.SkillInstall(false, "")
		require.NoError(t, err)
		require.Empty(t, result.Path)

		stderr := s.mockStderr.String()
		require.Contains(t, stderr, "npx skills add rwx-cloud/skills")
		require.Contains(t, stderr, "https://www.rwx.com/docs/ai")

		_, statErr := os.Stat(filepath.Join(s.tmp, ".agents", "skills", "rwx", "SKILL.md"))
		require.True(t, os.IsNotExist(statErr), "no SKILL.md should be written")
	})

	t.Run("interactive accepts word form 'repo'", func(t *testing.T) {
		s := setupSkillTest(t)
		s.config.StderrIsTTY = true
		s.mockStdin = bytes.NewBufferString("repo\n")
		s.config.Stdin = s.mockStdin

		s.mockAPI.MockGetSkillContent = func() (string, error) {
			return "---\nmetadata:\n  version: 2.0.0\n---\nSkill content\n", nil
		}

		t.Setenv("RWX_HIDE_SKILL_HINT", "1")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)

		result, err := s.service.SkillInstall(false, "")
		require.NoError(t, err)
		require.Equal(t, filepath.Join(s.tmp, ".agents", "skills", "rwx", "SKILL.md"), result.Path)
	})

	t.Run("interactive accepts word form 'user'", func(t *testing.T) {
		s := setupSkillTest(t)
		s.config.StderrIsTTY = true
		s.mockStdin = bytes.NewBufferString("user\n")
		s.config.Stdin = s.mockStdin

		s.mockAPI.MockGetSkillContent = func() (string, error) {
			t.Fatal("API should not be called when user picks user-level install")
			return "", nil
		}

		t.Setenv("RWX_HIDE_SKILL_HINT", "1")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)

		result, err := s.service.SkillInstall(false, "")
		require.NoError(t, err)
		require.Empty(t, result.Path)
		require.Contains(t, s.mockStderr.String(), "npx skills add rwx-cloud/skills")
	})

	t.Run("--yes with existing install confirms with renamed prompt copy", func(t *testing.T) {
		s := setupSkillTest(t)
		seedSkillFile(t, s.tmp, "1.0.0")

		s.mockAPI.MockGetSkillContent = func() (string, error) {
			return "---\nmetadata:\n  version: 2.0.0\n---\nSkill content\n", nil
		}

		t.Setenv("RWX_HIDE_SKILL_HINT", "1")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)

		result, err := s.service.SkillInstall(true, "")
		require.NoError(t, err)
		require.Equal(t, filepath.Join(s.tmp, ".agents", "skills", "rwx", "SKILL.md"), result.Path)

		stderr := s.mockStderr.String()
		require.Contains(t, stderr, "An existing")
		require.Contains(t, stderr, "installation was found")
		require.NotContains(t, stderr, "project level")
	})

	t.Run("interactive invalid choice errors", func(t *testing.T) {
		s := setupSkillTest(t)
		s.config.StderrIsTTY = true
		s.mockStdin = bytes.NewBufferString("nope\n")
		s.config.Stdin = s.mockStdin

		t.Setenv("RWX_HIDE_SKILL_HINT", "1")

		var err error
		s.service, err = cli.NewService(s.config)
		require.NoError(t, err)

		_, err = s.service.SkillInstall(false, "")
		require.Error(t, err)
	})
}
