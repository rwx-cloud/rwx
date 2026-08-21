#!/usr/bin/env bash
# Shared helpers for sandbox integration tests.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
RWX_CLI="${REPO_ROOT}/rwx"

start_sandbox() {
  local sandbox_config="${1:-${SCRIPT_DIR}/definitions/sandbox.yml}"
  local sandbox_result
  local exit_code=0
  sandbox_result=$("${RWX_CLI}" sandbox start "${sandbox_config}" --json --init ref=main --init "cli=${COMMIT_SHA}" --wait) || exit_code=$?

  if [ "$exit_code" -ne 0 ]; then
    echo "sandbox start failed with exit code ${exit_code}"
    echo "${sandbox_result}"
    exit 1
  fi

  local sandbox_url
  sandbox_url=$(echo "${sandbox_result}" | jq -r ".RunURL // empty")

  SANDBOX_RUN_ID=$(echo "${sandbox_result}" | jq -r ".RunID // empty")
  export SANDBOX_RUN_ID

  if [ -n "$sandbox_url" ]; then
    echo "Sandbox URL: ${sandbox_url}"
    echo "$sandbox_url" > "$RWX_LINKS/Sandbox Run"
  fi

  if [ -z "$SANDBOX_RUN_ID" ]; then
    echo "sandbox start did not return a run ID"
    exit 1
  fi
}

stop_sandbox() {
  "${RWX_CLI}" sandbox stop
}
