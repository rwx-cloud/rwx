#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/sandbox-helpers.sh"

ORIGINAL_HEAD=$(git rev-parse HEAD)

cleanup() {
  set +e
  stop_sandbox >/dev/null 2>&1
  git reset --hard "$ORIGINAL_HEAD" >/dev/null 2>&1
  rm -f uncommitted-test.txt unpushed-commit-test.txt
}
trap cleanup EXIT

rm -rf .rwx/sandboxes

echo "unpushed commit content" > unpushed-commit-test.txt
git add unpushed-commit-test.txt
git commit -m "unpushed local commit"
echo "uncommitted edit" > uncommitted-test.txt
uncommitted_sha=$(sha1sum uncommitted-test.txt | awk '{print $1}')

exec_exit_code=0
exec_result=$("${RWX_CLI}" sandbox exec \
  --json \
  "${SCRIPT_DIR}/definitions/sandbox.yml" \
  --init ref=main \
  --init "cli=${COMMIT_SHA}" \
  -- sh -c 'test "$(cat unpushed-commit-test.txt)" = "unpushed commit content" && test "$(cat uncommitted-test.txt)" = "uncommitted edit"') || exec_exit_code=$?
SANDBOX_RUN_ID=$(echo "$exec_result" | jq -r '.RunID // empty')
if [ -z "$SANDBOX_RUN_ID" ]; then
  echo "sandbox exec did not return a run ID"
  echo "$exec_result"
  exit 1
fi
if [ "$exec_exit_code" -ne 0 ]; then
  exit "$exec_exit_code"
fi

post_exec_sha=$(sha1sum uncommitted-test.txt | awk '{print $1}')
if [ "$uncommitted_sha" != "$post_exec_sha" ]; then
  echo "uncommitted-test.txt was lost during sandbox exec with unpushed commits (expected $uncommitted_sha, got $post_exec_sha)"
  exit 1
fi
