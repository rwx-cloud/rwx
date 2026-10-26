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
	// Codex sets CODEX_THREAD_ID on every shell command; the others depend on
	// the sandbox and exec path.
	{"codex", []string{"CODEX_THREAD_ID", "CODEX_SANDBOX", "CODEX_CI"}},
	{"gemini_cli", []string{"GEMINI_CLI"}},
	{"copilot_cli", []string{"COPILOT_CLI"}},
	// Amp sets AMP_THREAD_ID in its orbs.
	{"amp", []string{"AMP_THREAD_ID"}},
	{"opencode", []string{"OPENCODE"}},
	// AI_AGENT is a generic, agent-agnostic signal; only used when no specific
	// agent above matched. Its value is reported when it names the agent.
	{genericAgent, []string{"AI_AGENT"}},
	// Emdash launches other agents and sets EMDASH_TASK_ID in every task
	// session, so this catches agents not detected above. It is also set in
	// task terminals a human types into.
	{"emdash", []string{"EMDASH_TASK_ID"}},
}

const genericAgent = "agent"

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
			if !envSet(v) {
				continue
			}
			if e.agent == genericAgent {
				return genericAgentName(os.Getenv(v))
			}
			return e.agent
		}
	}
	return ""
}

// genericAgentName returns the agent named by a generic signal such as
// AI_AGENT=opencode, or "agent" when the value is only a boolean flag.
func genericAgentName(value string) string {
	name := strings.ToLower(strings.TrimSpace(value))
	switch name {
	case "1", "true", "yes":
		return genericAgent
	}
	return name
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
