#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/sandbox-helpers.sh"

cleanup() {
  git reset HEAD integration-test-local-change.txt 2>/dev/null || true
  rm -f integration-test-local-change.txt setup-artifact.txt
  "${RWX_CLI}" sandbox stop 2>/dev/null || true
}
trap cleanup EXIT

# Create and stage a local change so it's included in the API-level patch sent
# during sandbox start (untracked files are excluded from that patch).
echo "local-change-content" > integration-test-local-change.txt
git add integration-test-local-change.txt

# Run exec WITHOUT a prior sandbox start to trigger isNewSandbox=true.
# The sandbox auto-creates, and syncChangesToSandbox runs in baselineOnly mode:
# git apply --cached establishes refs/rwx-sync without touching the working tree.
"${RWX_CLI}" sandbox exec \
  "${SCRIPT_DIR}/definitions/sandbox-setup-sync.yml" \
  --init "ref=${COMMIT_SHA}" \
  -- sh -c 'test -f integration-test-local-change.txt || (echo "local change not found in sandbox" && exit 1)'

# Verify the setup artifact (created before rwx-sandbox in the sandbox task)
# was pulled back locally. This confirms refs/rwx-sync was correctly set to
# HEAD + local patch, making setup-artifact.txt visible in git diff refs/rwx-sync.
if [ ! -f setup-artifact.txt ]; then
  echo "FAIL: setup-artifact.txt was not pulled back from sandbox"
  exit 1
fi

actual=$(cat setup-artifact.txt)
if [ "$actual" != "setup-artifact-content" ]; then
  echo "FAIL: setup-artifact.txt has unexpected content: $actual"
  exit 1
fi

echo "PASS: local changes delivered to sandbox via patch; setup artifacts synced back locally"
