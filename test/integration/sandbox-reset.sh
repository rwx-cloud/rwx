#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/sandbox-helpers.sh"

start_sandbox
trap stop_sandbox EXIT

# Create a file in the sandbox
"${RWX_CLI}" sandbox exec -- sh -c 'echo "before-reset" > /tmp/reset-marker.txt'

# Verify the file exists
marker_check=$("${RWX_CLI}" sandbox exec --no-sync -- cat /tmp/reset-marker.txt 2>&1 | head -1)
if [ "$marker_check" != "before-reset" ]; then
  echo "Marker file not found in sandbox before reset"
  exit 1
fi

# Reset the sandbox
reset_output=$("${RWX_CLI}" sandbox reset "${SCRIPT_DIR}/definitions/sandbox.yml" --json --init ref=main --init "cli=${COMMIT_SHA}" --wait)
new_run_id=$(echo "$reset_output" | jq -r '.NewRunID // empty')

if [ -z "$new_run_id" ]; then
  echo "Reset did not return a NewRunID"
  echo "$reset_output"
  exit 1
fi

# After reset, the marker file should not exist in the fresh sandbox
exit_code=0
"${RWX_CLI}" sandbox exec --no-sync -- cat /tmp/reset-marker.txt 2>/dev/null || exit_code=$?

if [ "$exit_code" -eq 0 ]; then
  echo "Marker file still exists after sandbox reset - sandbox was not re-provisioned"
  exit 1
fi
