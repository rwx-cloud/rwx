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

	t.Run("codex via thread id", func(t *testing.T) {
		clearAgentEnv(t)
		t.Setenv("CODEX_THREAD_ID", "019a-thread")
		require.Equal(t, "codex", detectAgent())
	})

	t.Run("emdash", func(t *testing.T) {
		clearAgentEnv(t)
		t.Setenv("EMDASH_TASK_ID", "task-1")
		require.Equal(t, "emdash", detectAgent())
	})

	t.Run("detected agent wins over emdash", func(t *testing.T) {
		clearAgentEnv(t)
		t.Setenv("EMDASH_TASK_ID", "task-1")
		t.Setenv("CLAUDECODE", "1")
		require.Equal(t, "claude_code", detectAgent())
	})

	t.Run("named AI_AGENT wins over emdash", func(t *testing.T) {
		clearAgentEnv(t)
		t.Setenv("EMDASH_TASK_ID", "task-1")
		t.Setenv("AI_AGENT", "kilocode")
		require.Equal(t, "kilocode", detectAgent())
	})

	t.Run("generic AI_AGENT", func(t *testing.T) {
		clearAgentEnv(t)
		t.Setenv("AI_AGENT", "true")
		require.Equal(t, "agent", detectAgent())
	})

	t.Run("generic AI_AGENT with a name", func(t *testing.T) {
		clearAgentEnv(t)
		t.Setenv("AI_AGENT", " Some-Agent ")
		require.Equal(t, "some-agent", detectAgent())
	})

	t.Run("amp", func(t *testing.T) {
		clearAgentEnv(t)
		t.Setenv("AMP_THREAD_ID", "T-123")
		require.Equal(t, "amp", detectAgent())
	})

	t.Run("amp via AI_AGENT", func(t *testing.T) {
		clearAgentEnv(t)
		t.Setenv("AI_AGENT", "amp")
		require.Equal(t, "amp", detectAgent())
	})

	t.Run("opencode", func(t *testing.T) {
		clearAgentEnv(t)
		t.Setenv("OPENCODE", "1")
		t.Setenv("AI_AGENT", "opencode")
		require.Equal(t, "opencode", detectAgent())
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

	t.Run("rwx", func(t *testing.T) {
		clearCIEnv(t)
		t.Setenv("RWX", "true")
		require.Equal(t, "rwx", detectCI())
	})

	t.Run("specific CI wins over generic", func(t *testing.T) {
		clearCIEnv(t)
		t.Setenv("CI", "1")
		t.Setenv("BUILDKITE", "true")
		require.Equal(t, "buildkite", detectCI())
	})

	t.Run("rwx wins over generic", func(t *testing.T) {
		clearCIEnv(t)
		t.Setenv("CI", "1")
		t.Setenv("RWX", "true")
		require.Equal(t, "rwx", detectCI())
	})

	t.Run("falsy values are not detected", func(t *testing.T) {
		clearCIEnv(t)
		t.Setenv("CI", "false")
		require.Equal(t, "", detectCI())
	})
}
