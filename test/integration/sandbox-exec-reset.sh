#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/sandbox-helpers.sh"

start_sandbox
trap stop_sandbox EXIT

# Create a sentinel file in the existing sandbox
"${RWX_CLI}" sandbox exec -- sh -c 'echo "sentinel" > /tmp/exec-reset-marker.txt'

# Verify the sentinel exists before reset
marker=$("${RWX_CLI}" sandbox exec --no-sync -- cat /tmp/exec-reset-marker.txt)
if [ "$marker" != "sentinel" ]; then
  echo "ERROR: Sentinel file not found in sandbox before exec --reset"
  exit 1
fi

# exec --reset should stop the existing sandbox, start a fresh one, and run the command
"${RWX_CLI}" sandbox exec --reset --init ref=main --init "cli=${COMMIT_SHA}" "${SCRIPT_DIR}/definitions/sandbox.yml" -- sh -c 'echo "exec-reset-complete"'

# In the fresh sandbox, the sentinel file should not exist
exit_code=0
"${RWX_CLI}" sandbox exec --no-sync -- cat /tmp/exec-reset-marker.txt 2>/dev/null || exit_code=$?

if [ "$exit_code" -eq 0 ]; then
  echo "ERROR: Sentinel file still exists after exec --reset - sandbox was not re-provisioned"
  exit 1
fi
