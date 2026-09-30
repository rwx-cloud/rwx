#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/sandbox-helpers.sh"

SANDBOX_CONFIG="${SCRIPT_DIR}/definitions/sandbox-setup-sync.yml"

PUSH_OLD="CONTRIBUTING.md"
PUSH_NEW="CONTRIBUTING-renamed.md"
PULL_OLD="LICENSE"
PULL_NEW="LICENSE-renamed"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

cleanup() {
  rm -f "${PUSH_NEW}" "${PULL_NEW}"
  git restore "${PUSH_OLD}" "${PULL_OLD}" 2>/dev/null || true
  rm -f setup-artifact.txt
  "${RWX_CLI}" sandbox stop 2>/dev/null || true
}
trap cleanup EXIT

echo "Scenario: a local file rename syncs into a fresh sandbox"
mv "${PUSH_OLD}" "${PUSH_NEW}"
"${RWX_CLI}" sandbox exec "${SANDBOX_CONFIG}" --init "ref=${COMMIT_SHA}" \
  -- sh -c "test ! -e '${PUSH_OLD}' && test -e '${PUSH_NEW}'"
git restore "${PUSH_OLD}"
rm -f "${PUSH_NEW}"
echo "PASS: local rename synced into the sandbox"

echo "Scenario: a rename performed inside the sandbox is pulled back locally"
"${RWX_CLI}" sandbox exec "${SANDBOX_CONFIG}" --init "ref=${COMMIT_SHA}" \
  -- sh -c "git mv '${PULL_OLD}' '${PULL_NEW}'"
[ ! -e "${PULL_OLD}" ] || fail "${PULL_OLD} should have been removed locally after the sandbox rename"
[ -e "${PULL_NEW}" ] || fail "${PULL_NEW} was not pulled back from the sandbox"
echo "PASS: sandbox-side rename pulled back locally"
