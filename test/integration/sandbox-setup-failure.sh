#!/usr/bin/env bash
# Verifies that setup failure output is surfaced in stderr when a sandbox run
# fails before reaching the sandbox task.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
RWX_CLI="${REPO_ROOT}/rwx"

stderr_output=$("${RWX_CLI}" sandbox exec \
  "${SCRIPT_DIR}/definitions/sandbox-setup-failure.yml" \
  -- echo hello 2>&1 >/dev/null) || true

if ! echo "$stderr_output" | grep -q "Failed task"; then
  echo "ERROR: Expected setup failure prompt in stderr but did not find it"
  echo "stderr was: $stderr_output"
  exit 1
fi

if ! echo "$stderr_output" | grep -q "preflight"; then
  echo "ERROR: Expected failing task name in stderr but did not find it"
  echo "stderr was: $stderr_output"
  exit 1
fi

echo "OK: setup failure prompt was surfaced in stderr"
