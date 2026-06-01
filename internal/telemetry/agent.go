package telemetry

import (
	"os"
	"strings"
)

// agentEnvVars maps a coding agent to the environment variables whose presence
// signals it. There is no cross-agent standard yet (akin to CI=true), so this
// is a best-effort, additive list.
//
// Order matters: the first agent with a matching variable wins, so specific
// agents come before the generic AI_AGENT catch-all.
var agentEnvVars = []struct {
	agent   string
	envVars []string
}{
	// Claude Code sets CLAUDECODE=1 in every subprocess it spawns.
	// https://code.claude.com/docs/en/env-vars
	{"claude_code", []string{"CLAUDECODE", "CLAUDE_CODE"}},
	{"cursor", []string{"CURSOR_AGENT"}},
	{"codex", []string{"CODEX_SANDBOX", "CODEX_CI"}},
	{"gemini_cli", []string{"GEMINI_CLI"}},
	{"copilot_cli", []string{"COPILOT_CLI"}},
	// AI_AGENT is a generic, agent-agnostic signal; only used when no specific
	// agent above matched.
	{"agent", []string{"AI_AGENT"}},
}

// detectAgent returns the name of the coding agent the CLI appears to be
// running inside, or "" when none is detected.
func detectAgent() string {
	for _, e := range agentEnvVars {
		for _, v := range e.envVars {
			if envSet(v) {
				return e.agent
			}
		}
	}
	return ""
}

// envSet reports whether an environment variable is present and not explicitly
// disabled. We accept any non-falsy value rather than requiring "1"/"true"
// because some agents use other values (e.g. Codex sets CODEX_SANDBOX=seatbelt).
func envSet(name string) bool {
	v, ok := os.LookupEnv(name)
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "0", "false":
		return false
	}
	return true
}
