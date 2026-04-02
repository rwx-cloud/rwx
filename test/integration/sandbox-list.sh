#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/sandbox-helpers.sh"

start_sandbox
trap stop_sandbox EXIT

# Verify the sandbox appears in the list
list_output=$("${RWX_CLI}" sandbox list --json)
sandbox_count=$(echo "$list_output" | jq '.Sandboxes | length')

if [ "$sandbox_count" -lt 1 ]; then
  echo "Expected at least 1 sandbox in list, got $sandbox_count"
  echo "$list_output"
  exit 1
fi

# Verify at least one sandbox has a non-empty RunID
run_id_count=$(echo "$list_output" | jq '[.Sandboxes[] | select((.RunID // "") != "")] | length')
if [ "$run_id_count" -lt 1 ]; then
  echo "Expected at least one sandbox with a non-empty RunID"
  echo "$list_output"
  exit 1
fi
