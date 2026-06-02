package telemetry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// clearAgentEnv unsets every env var that detectAgent inspects so a test starts
// from a known-empty environment. t.Setenv restores prior values on cleanup.
func clearAgentEnv(t *testing.T) {
	t.Helper()
	for _, e := range agentEnvVars {
		for _, v := range e.envVars {
			t.Setenv(v, "")
		}
	}
}

// clearCIEnv unsets every env var that detectCI inspects so a test starts
// from a known-empty environment. t.Setenv restores prior values on cleanup.
func clearCIEnv(t *testing.T) {
	t.Helper()
	for _, e := range ciEnvVars {
		for _, v := range e.envVars {
			t.Setenv(v, "")
		}
	}
}

func TestDetectAgent(t *testing.T) {
	t.Run("none detected", func(t *testing.T) {
		clearAgentEnv(t)
		require.Equal(t, "", detectAgent())
	})

	t.Run("claude code", func(t *testing.T) {
		clearAgentEnv(t)
		t.Setenv("CLAUDECODE", "1")
		require.Equal(t, "claude_code", detectAgent())
	})

	t.Run("codex via seatbelt value", func(t *testing.T) {
		clearAgentEnv(t)
		t.Setenv("CODEX_SANDBOX", "seatbelt")
		require.Equal(t, "codex", detectAgent())
	})

	t.Run("generic AI_AGENT", func(t *testing.T) {
		clearAgentEnv(t)
		t.Setenv("AI_AGENT", "true")
		require.Equal(t, "agent", detectAgent())
	})

	t.Run("specific agent wins over generic", func(t *testing.T) {
		clearAgentEnv(t)
		t.Setenv("AI_AGENT", "1")
		t.Setenv("CLAUDECODE", "1")
		require.Equal(t, "claude_code", detectAgent())
	})

	t.Run("falsy values are not detected", func(t *testing.T) {
		clearAgentEnv(t)
		t.Setenv("AI_AGENT", "false")
		require.Equal(t, "", detectAgent())
	})
}

func TestDetectCI(t *testing.T) {
	t.Run("none detected", func(t *testing.T) {
		clearCIEnv(t)
		require.Equal(t, "", detectCI())
	})

	t.Run("generic CI", func(t *testing.T) {
		clearCIEnv(t)
		t.Setenv("CI", "true")
		require.Equal(t, "ci", detectCI())
	})

	t.Run("github actions", func(t *testing.T) {
		clearCIEnv(t)
		t.Setenv("GITHUB_ACTIONS", "true")
		require.Equal(t, "github_actions", detectCI())
	})

	t.Run("specific CI wins over generic", func(t *testing.T) {
		clearCIEnv(t)
		t.Setenv("CI", "1")
		t.Setenv("BUILDKITE", "true")
		require.Equal(t, "buildkite", detectCI())
	})

	t.Run("falsy values are not detected", func(t *testing.T) {
		clearCIEnv(t)
		t.Setenv("CI", "false")
		require.Equal(t, "", detectCI())
	})
}
