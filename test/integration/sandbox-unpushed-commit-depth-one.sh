#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/sandbox-helpers.sh"

ORIGINAL_HEAD=$(git rev-parse HEAD)
SANDBOX_RUN_ID=""

cleanup() {
  set +e
  if [ -n "$SANDBOX_RUN_ID" ]; then
    "${RWX_CLI}" sandbox stop --id "$SANDBOX_RUN_ID" >/dev/null 2>&1
  fi
  git reset --hard "$ORIGINAL_HEAD" >/dev/null 2>&1
  rm -f unpushed-depth-one-test.txt
}
trap cleanup EXIT

rm -rf .rwx/sandboxes

if [ "$(git rev-parse --is-shallow-repository)" = "true" ]; then
  git fetch --unshallow origin
fi

sandbox_result=$(
  "${RWX_CLI}" sandbox start \
    "${SCRIPT_DIR}/definitions/sandbox-depth-one.yml" \
    --json \
    --init "ref=${COMMIT_SHA}" \
    --wait
)
SANDBOX_RUN_ID=$(echo "$sandbox_result" | jq -r ".RunID")

sandbox_url=$(echo "$sandbox_result" | jq -r ".RunURL // empty")
if [ -n "$sandbox_url" ]; then
  echo "Sandbox URL: ${sandbox_url}"
  echo "$sandbox_url" > "$RWX_LINKS/Sandbox Run"
fi
if [ -z "$SANDBOX_RUN_ID" ] || [ "$SANDBOX_RUN_ID" = "null" ]; then
  echo "$sandbox_result"
  echo "sandbox start did not return a run id"
  exit 1
fi

echo "unpushed commit content" > unpushed-depth-one-test.txt
git add unpushed-depth-one-test.txt
git commit -m "unpushed local commit"

output=$(
  "${RWX_CLI}" sandbox exec \
    --id "$SANDBOX_RUN_ID" \
    -- sh -c \
    'test "$(git rev-parse --is-shallow-repository)" = "true" &&
      test "$(cat unpushed-depth-one-test.txt)" = "unpushed commit content" &&
      echo "depth-one sandbox command ran"'
)
echo "$output"

if ! echo "$output" | grep -q "depth-one sandbox command ran"; then
  echo "sandbox command did not run after synchronizing the unpushed commit"
  exit 1
fi
