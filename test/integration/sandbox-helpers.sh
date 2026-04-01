#!/usr/bin/env bash
# Shared helpers for sandbox integration tests.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
RWX_CLI="${REPO_ROOT}/rwx"

start_sandbox() {
  local sandbox_result
  local exit_code=0
  sandbox_result=$("${RWX_CLI}" sandbox start "${SCRIPT_DIR}/definitions/sandbox.yml" --json --init ref=main --init "cli=${COMMIT_SHA}" --wait) || exit_code=$?

  local sandbox_url
  sandbox_url=$(echo "${sandbox_result}" | jq -r ".RunURL // empty")
  if [ -n "$sandbox_url" ]; then
    echo "Sandbox URL: ${sandbox_url}"
    echo "$sandbox_url" > "$RWX_LINKS/Sandbox Run"
  fi

  if [ "$exit_code" -ne 0 ]; then
    echo "sandbox start failed with exit code ${exit_code}"
    echo "${sandbox_result}"
    exit 1
  fi
}

stop_sandbox() {
  "${RWX_CLI}" sandbox stop
}
