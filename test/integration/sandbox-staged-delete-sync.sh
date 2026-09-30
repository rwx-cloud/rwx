#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/sandbox-helpers.sh"

DELETED_FILE="CONTRIBUTING.md"
EDITED_FILE="README.md"
EDIT_MARKER="integration staged delete sync marker"

cleanup() {
  git restore --staged "${DELETED_FILE}" 2>/dev/null || true
  git restore "${DELETED_FILE}" "${EDITED_FILE}" 2>/dev/null || true
  rm -f setup-artifact.txt
  "${RWX_CLI}" sandbox stop 2>/dev/null || true
}
trap cleanup EXIT

git rm "${DELETED_FILE}"
printf '\n%s\n' "${EDIT_MARKER}" >> "${EDITED_FILE}"

"${RWX_CLI}" sandbox exec \
  "${SCRIPT_DIR}/definitions/sandbox-setup-sync.yml" \
  --init "ref=${COMMIT_SHA}" \
  -- sh -c "test ! -e '${DELETED_FILE}' && grep -q '${EDIT_MARKER}' '${EDITED_FILE}'"

echo "PASS: fresh sandbox sync handled staged deletion with unstaged edit"
