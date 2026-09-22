#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/sandbox-helpers.sh"
export RWX_EXPERIMENTAL=true

cleanup() {
  if [ -n "${IMPLICIT_STDERR:-}" ]; then
    rm -f "$IMPLICIT_STDERR"
  fi
  if [ -n "${SANDBOX_RUN_ID:-}" ]; then
    stop_sandbox
  fi
}
trap cleanup EXIT

git switch -c "sandbox-history-$(date +%s)-$$"

sandbox_config="${SCRIPT_DIR}/definitions/sandbox.yml"
expected_config=$(realpath "$sandbox_config")
start_sandbox "$sandbox_config"
historical_run_id="$SANDBOX_RUN_ID"

stop_sandbox
SANDBOX_RUN_ID=""

IMPLICIT_STDERR=$(mktemp)
implicit_output=$("${RWX_CLI}" sandbox exec --json --init "commit-sha=${COMMIT_SHA}" -- true 2>"$IMPLICIT_STDERR")
implicit_warning=$(cat "$IMPLICIT_STDERR")
if ! echo "$implicit_warning" | grep -q "Starting a new sandbox with the default definition"; then
  echo "Expected implicit sandbox exec to warn about starting the default definition"
  echo "$implicit_warning"
  exit 1
fi
SANDBOX_RUN_ID=$(echo "$implicit_output" | jq -r '.RunID // empty')
if [ -z "$SANDBOX_RUN_ID" ]; then
  echo "Implicit sandbox exec did not return a run ID"
  echo "$implicit_output"
  exit 1
fi

default_config=$(realpath "${SCRIPT_DIR}/../../.rwx/sandbox.yml")
selected_config=$("${RWX_CLI}" sandbox list --json | jq -r --arg id "$SANDBOX_RUN_ID" '.Sandboxes[] | select(.RunID == $id) | .ConfigFile')
if [ "$selected_config" != "$default_config" ]; then
  echo "Expected implicit sandbox exec to use $default_config, got $selected_config"
  exit 1
fi

stop_sandbox
SANDBOX_RUN_ID=""

exit_code=0
inactive_output=$("${RWX_CLI}" sandbox background logs "$sandbox_config" --key historical-selection --json 2>&1) || exit_code=$?
if [ "$exit_code" -eq 0 ]; then
  echo "Expected explicit sandbox background logs to reject the inactive historical sandbox"
  echo "$inactive_output"
  exit 1
fi
if ! echo "$inactive_output" | grep -q "is no longer active"; then
  echo "Expected explicit sandbox background logs to report that the historical sandbox is inactive"
  echo "$inactive_output"
  exit 1
fi

replacement_output=$("${RWX_CLI}" sandbox exec "$sandbox_config" --json --init ref=main --init "cli=${COMMIT_SHA}" -- true)
SANDBOX_RUN_ID=$(echo "$replacement_output" | jq -r '.RunID // empty')
if [ -z "$SANDBOX_RUN_ID" ]; then
  echo "Historical sandbox replacement did not return a run ID"
  echo "$replacement_output"
  exit 1
fi
if [ "$SANDBOX_RUN_ID" = "$historical_run_id" ]; then
  echo "Historical sandbox selection reused the inactive run"
  exit 1
fi

selected_config=$("${RWX_CLI}" sandbox list --json | jq -r --arg id "$SANDBOX_RUN_ID" '.Sandboxes[] | select(.RunID == $id) | .ConfigFile')
if [ "$selected_config" != "$expected_config" ]; then
  echo "Expected replacement to use $expected_config, got $selected_config"
  exit 1
fi
