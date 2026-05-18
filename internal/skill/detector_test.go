package skill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSkillVersion(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name: "valid frontmatter with version",
			content: `---
metadata:
  version: "1.2.0"
---
# RWX Skill
`,
			expected: "1.2.0",
		},
		{
			name:     "no frontmatter",
			content:  "# RWX Skill\nSome content",
			expected: "",
		},
		{
			name: "frontmatter without version",
			content: `---
metadata:
  name: rwx
---
# RWX Skill
`,
			expected: "",
		},
		{
			name:     "empty content",
			content:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version := parseSkillVersion(tt.content)
			require.Equal(t, tt.expected, version)
		})
	}
}

func TestParseSkillVersionWithoutMetadata(t *testing.T) {
	content := `---
name: rwx
description: Some skill
---
# Content
`
	version := parseSkillVersion(content)
	require.Equal(t, "", version)
}

func TestIsDetected(t *testing.T) {
	require.True(t, IsDetected(Installation{Detected: true, Version: "1.0.0"}))
	require.True(t, IsDetected(Installation{Detected: true}))
	require.False(t, IsDetected(Installation{Detected: false}))
}

// seedSkillMD creates a SKILL.md with the given version at the specified path.
func seedSkillMD(t *testing.T, path, version string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	content := "---\nmetadata:\n  version: \"" + version + "\"\n---\nSkill content\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestDetect(t *testing.T) {
	t.Run("no installations", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Chdir(tmp)

		result, err := Detect()
		require.NoError(t, err)
		require.False(t, result.AnyFound)
	})

	t.Run("agents user only", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Chdir(tmp)

		seedSkillMD(t, filepath.Join(tmp, ".agents", "skills", "rwx", "SKILL.md"), "1.0.0")

		result, err := Detect()
		require.NoError(t, err)
		require.True(t, result.AnyFound)

		var found *Installation
		for _, inst := range result.Installations {
			if inst.Detected && inst.Source == "agents" {
				found = &inst
				break
			}
		}
		require.NotNil(t, found)
		require.Equal(t, "user", found.Scope)
		require.Equal(t, "agents", found.Source)
		require.Equal(t, "1.0.0", found.Version)
	})

	t.Run("agents repo only", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)

		projectDir := filepath.Join(tmp, "myproject")
		require.NoError(t, os.MkdirAll(projectDir, 0o755))
		t.Chdir(projectDir)

		seedSkillMD(t, filepath.Join(projectDir, ".agents", "skills", "rwx", "SKILL.md"), "2.0.0")

		result, err := Detect()
		require.NoError(t, err)
		require.True(t, result.AnyFound)

		var found *Installation
		for _, inst := range result.Installations {
			if inst.Detected && inst.Source == "agents" {
				found = &inst
				break
			}
		}
		require.NotNil(t, found)
		require.Equal(t, "repo", found.Scope)
		require.Equal(t, "agents", found.Source)
		require.Equal(t, "2.0.0", found.Version)
	})

	t.Run("marketplace only", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Chdir(tmp)

		seedSkillMD(t, filepath.Join(tmp, ".claude", "plugins", "marketplaces", "rwx", "plugins", "rwx", "skills", "rwx", "SKILL.md"), "0.1.3")

		result, err := Detect()
		require.NoError(t, err)
		require.True(t, result.AnyFound)

		var found *Installation
		for _, inst := range result.Installations {
			if inst.Detected && inst.Source == "marketplace" {
				found = &inst
				break
			}
		}
		require.NotNil(t, found)
		require.Equal(t, "user", found.Scope)
		require.Equal(t, "marketplace", found.Source)
		require.Equal(t, "0.1.3", found.Version)
	})

	t.Run("both agents and marketplace", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Chdir(tmp)

		seedSkillMD(t, filepath.Join(tmp, ".agents", "skills", "rwx", "SKILL.md"), "1.0.0")
		seedSkillMD(t, filepath.Join(tmp, ".claude", "plugins", "marketplaces", "rwx", "plugins", "rwx", "skills", "rwx", "SKILL.md"), "0.1.3")

		result, err := Detect()
		require.NoError(t, err)
		require.True(t, result.AnyFound)

		sources := make(map[string]bool)
		for _, inst := range result.Installations {
			if inst.Detected {
				sources[inst.Source] = true
			}
		}
		require.True(t, sources["agents"])
		require.True(t, sources["marketplace"])
	})
}
