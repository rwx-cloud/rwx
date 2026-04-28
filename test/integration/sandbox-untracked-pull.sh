#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/sandbox-helpers.sh"

UNTRACKED_FILE="integration-test-untracked-survives.txt"
UNTRACKED_CONTENT="untracked-survives-content"

cleanup() {
  rm -f "${UNTRACKED_FILE}" setup-artifact.txt
  "${RWX_CLI}" sandbox stop 2>/dev/null || true
}
trap cleanup EXIT

# Create an untracked file. Deliberately do NOT `git add` it — this exercises
# the regression where syncChangesToSandbox in baselineOnly mode applies the
# patch with `git apply --cached`. New-file additions land only in the index,
# never in the working tree, so refs/rwx-sync ends up ahead of the sandbox WT.
# On pull, `git diff refs/rwx-sync` reports the file as a deletion, which the
# CLI then applies locally, removing the user's untracked file.
echo "${UNTRACKED_CONTENT}" > "${UNTRACKED_FILE}"

# Run exec WITHOUT a prior sandbox start to trigger isNewSandbox=true.
"${RWX_CLI}" sandbox exec \
  "${SCRIPT_DIR}/definitions/sandbox-setup-sync.yml" \
  --init "ref=${COMMIT_SHA}" \
  -- sh -c 'true'

if [ ! -f "${UNTRACKED_FILE}" ]; then
  echo "FAIL: local untracked file ${UNTRACKED_FILE} was deleted by sandbox patch-back"
  exit 1
fi

actual=$(cat "${UNTRACKED_FILE}")
if [ "${actual}" != "${UNTRACKED_CONTENT}" ]; then
  echo "FAIL: local untracked file ${UNTRACKED_FILE} has unexpected content: ${actual}"
  exit 1
fi

echo "PASS: untracked local file survived sandbox exec"
