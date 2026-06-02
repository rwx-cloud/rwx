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

// ciEnvVars maps CI systems to environment variables whose presence signals
// them. Order matters: provider-specific signals win over the generic CI=true
// convention.
var ciEnvVars = []struct {
	ci      string
	envVars []string
}{
	{"github_actions", []string{"GITHUB_ACTIONS"}},
	{"gitlab_ci", []string{"GITLAB_CI"}},
	{"circleci", []string{"CIRCLECI"}},
	{"buildkite", []string{"BUILDKITE"}},
	{"jenkins", []string{"JENKINS_URL", "JENKINS_HOME"}},
	{"travis_ci", []string{"TRAVIS"}},
	{"azure_pipelines", []string{"TF_BUILD"}},
	{"bitbucket_pipelines", []string{"BITBUCKET_BUILD_NUMBER"}},
	{"teamcity", []string{"TEAMCITY_VERSION"}},
	{"rwx", []string{"RWX"}},
	{"ci", []string{"CI"}},
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

// detectCI returns the name of the CI environment the CLI appears to be
// running inside, or "" when none is detected.
func detectCI() string {
	for _, e := range ciEnvVars {
		for _, v := range e.envVars {
			if envSet(v) {
				return e.ci
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
