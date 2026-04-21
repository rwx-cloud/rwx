#!/usr/bin/env bash
# Verifies that setup failure output is surfaced in stdout when a sandbox run
# fails before reaching the sandbox task.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
RWX_CLI="${REPO_ROOT}/rwx"

stdout_output=$("${RWX_CLI}" sandbox exec \
  "${SCRIPT_DIR}/definitions/sandbox-setup-failure.yml" \
  -- echo hello || true)

if ! echo "$stdout_output" | grep -q "Failed task"; then
  echo "ERROR: Expected setup failure prompt in stdout but did not find it"
  echo "stdout was: $stdout_output"
  exit 1
fi

if ! echo "$stdout_output" | grep -q "preflight"; then
  echo "ERROR: Expected failing task name in stdout but did not find it"
  echo "stdout was: $stdout_output"
  exit 1
fi

echo "OK: setup failure prompt was surfaced in stdout"
