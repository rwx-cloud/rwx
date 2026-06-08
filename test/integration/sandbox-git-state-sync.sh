#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/sandbox-helpers.sh"

SANDBOX_CONFIG="${SCRIPT_DIR}/definitions/sandbox.yml"
SANDBOX_RUN_ID=""

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

require_clean_worktree() {
  git diff --quiet || fail "sandbox git-state integration test requires a clean working tree"
  git diff --cached --quiet || fail "sandbox git-state integration test requires a clean index"
  if [ -n "$(git ls-files --others --exclude-standard)" ]; then
    fail "sandbox git-state integration test requires no untracked files"
  fi
}

commit_file() {
  local file="$1"
  local content="$2"
  local message="$3"

  printf '%s\n' "$content" > "$file"
  git add "$file"
  git -c user.email="sandbox-integration@example.com" -c user.name="Sandbox Integration" commit -m "$message" >/dev/null
}

commit_lfs_file() {
  local file="$1"
  local content="$2"
  local message="$3"

  git lfs track "$file" >/dev/null
  printf '%s\n' "$content" > "$file"
  git add .gitattributes "$file"
  git -c user.email="sandbox-integration@example.com" -c user.name="Sandbox Integration" commit -m "$message" >/dev/null
}

require_git_lfs() {
  git lfs version >/dev/null 2>&1 || fail "sandbox LFS integration scenarios require git-lfs"
}

ensure_sandbox_git_lfs() {
  "${RWX_CLI}" sandbox exec --id "$SANDBOX_RUN_ID" -- sh -c '
    set -e
    if ! /usr/bin/git lfs version >/dev/null 2>&1; then
      sudo apt-get update >/dev/null
      sudo apt-get install -y git-lfs >/dev/null
    fi
    /usr/bin/git lfs install --local >/dev/null
  '
}

run_and_capture_output() {
  local output_file="$1"
  shift

  set +e
  if command -v timeout >/dev/null 2>&1; then
    timeout 300 "$@" 2>&1 | tee "$output_file"
  else
    "$@" 2>&1 | tee "$output_file"
  fi
  local exit_code=${PIPESTATUS[0]}
  set -e

  return "$exit_code"
}

assert_sandbox_head_matches() {
  local expected
  local actual

  expected=$(git rev-parse HEAD)
  actual=$("${RWX_CLI}" sandbox exec --id "$SANDBOX_RUN_ID" -- git rev-parse HEAD | awk 'NR==1{print $1}')
  if [ "$actual" != "$expected" ]; then
    fail "sandbox HEAD mismatch (local: $expected, sandbox: $actual)"
  fi
}

assert_sandbox_file_content() {
  local file="$1"
  local expected="$2"
  local actual

  actual=$("${RWX_CLI}" sandbox exec --id "$SANDBOX_RUN_ID" -- cat "$file" | awk 'NR==1{print; exit}')
  if [ "$actual" != "$expected" ]; then
    fail "${file} content mismatch in sandbox (expected: ${expected}, actual: ${actual})"
  fi
}

assert_sandbox_file_missing() {
  local file="$1"

  if "${RWX_CLI}" sandbox exec --id "$SANDBOX_RUN_ID" -- test -e "$file"; then
    fail "${file} exists in sandbox but should have been removed after local git state changed"
  fi
}

assert_local_file_content() {
  local file="$1"
  local expected="$2"

  if [ ! -f "$file" ]; then
    fail "${file} was not pulled from sandbox"
  fi

  local actual
  actual=$(cat "$file")
  if [ "$actual" != "$expected" ]; then
    fail "${file} content mismatch locally (expected: ${expected}, actual: ${actual})"
  fi
}

start_git_state_sandbox() {
  if [ -n "$SANDBOX_RUN_ID" ]; then
    "${RWX_CLI}" sandbox stop --id "$SANDBOX_RUN_ID" >/dev/null 2>&1 || true
    SANDBOX_RUN_ID=""
  fi

  local sandbox_result
  sandbox_result=$("${RWX_CLI}" sandbox start "$SANDBOX_CONFIG" --json --init ref=main --init "cli=${COMMIT_SHA}" --wait)
  SANDBOX_RUN_ID=$(echo "$sandbox_result" | jq -r ".RunID")

  local sandbox_url
  sandbox_url=$(echo "$sandbox_result" | jq -r ".RunURL // empty")
  if [ -n "$sandbox_url" ]; then
    echo "Sandbox URL: ${sandbox_url}"
    echo "$sandbox_url" > "$RWX_LINKS/Sandbox Run"
  fi
  if [ -z "$SANDBOX_RUN_ID" ] || [ "$SANDBOX_RUN_ID" = "null" ]; then
    echo "$sandbox_result"
    fail "sandbox start did not return a run id"
  fi
  rm -rf .rwx/sandboxes
}

require_clean_worktree

ORIGINAL_HEAD=$(git rev-parse HEAD)
ORIGINAL_BRANCH=$(git branch --show-current)
TEST_ID="rwx-sandbox-git-state-$$"
TEMP_BRANCHES=(
  "${TEST_ID}-push"
  "${TEST_ID}-feature-rebase"
  "${TEST_ID}-main-rebase"
  "${TEST_ID}-feature-merge"
  "${TEST_ID}-main-merge"
  "${TEST_ID}-force-move"
  "${TEST_ID}-sandbox-created"
  "${TEST_ID}-detached-source"
  "${TEST_ID}-lfs-push"
  "${TEST_ID}-lfs-patch"
  "${TEST_ID}-dirty-state"
)
TEST_FILES=(
  integration-pushed-commit.txt
  integration-feature-before-rebase.txt
  integration-main-after-rebase.txt
  integration-feature-merge.txt
  integration-main-merge.txt
  integration-force-move-old.txt
  integration-force-move-new.txt
  integration-sandbox-created-survives.txt
  integration-local-after-sandbox-created.txt
  integration-detached-head.txt
  integration-lfs-push.bin
  integration-lfs-patch.bin
  integration-staged-state.txt
)

cleanup() {
  set +e
  if [ -n "$SANDBOX_RUN_ID" ]; then
    "${RWX_CLI}" sandbox stop --id "$SANDBOX_RUN_ID" >/dev/null 2>&1
  fi
  git reset --hard "$ORIGINAL_HEAD" >/dev/null 2>&1
  rm -f "${TEST_FILES[@]}"
  if [ -n "$ORIGINAL_BRANCH" ]; then
    git switch "$ORIGINAL_BRANCH" >/dev/null 2>&1
  else
    git switch --detach "$ORIGINAL_HEAD" >/dev/null 2>&1
  fi
  if ! git cat-file -e "$ORIGINAL_HEAD":.gitattributes >/dev/null 2>&1; then
    rm -f .gitattributes
  fi
  git branch -D "${TEMP_BRANCHES[@]}" >/dev/null 2>&1
}
trap cleanup EXIT

start_git_state_sandbox
require_clean_worktree

echo "Scenario: unpushed local commits are pushed and become sandbox HEAD"
git switch -C "${TEST_ID}-push" "$ORIGINAL_HEAD" >/dev/null
commit_file "integration-pushed-commit.txt" "pushed commit content" "integration pushed commit"
assert_sandbox_head_matches
assert_sandbox_file_content "integration-pushed-commit.txt" "pushed commit content"

echo "Scenario: rebased branch carries newer main commits into reused sandbox"
git switch -C "${TEST_ID}-feature-rebase" "$ORIGINAL_HEAD" >/dev/null
commit_file "integration-feature-before-rebase.txt" "feature before rebase" "integration feature before rebase"
assert_sandbox_file_content "integration-feature-before-rebase.txt" "feature before rebase"

git switch -C "${TEST_ID}-main-rebase" "$ORIGINAL_HEAD" >/dev/null
commit_file "integration-main-after-rebase.txt" "main advanced before rebase" "integration main advance before rebase"

git switch "${TEST_ID}-feature-rebase" >/dev/null
git rebase "${TEST_ID}-main-rebase" >/dev/null
assert_sandbox_head_matches
assert_sandbox_file_content "integration-main-after-rebase.txt" "main advanced before rebase"
assert_sandbox_file_content "integration-feature-before-rebase.txt" "feature before rebase"

echo "Scenario: merge commits bring main changes into sandbox"
git switch -C "${TEST_ID}-main-merge" "$ORIGINAL_HEAD" >/dev/null
commit_file "integration-main-merge.txt" "main merged into feature" "integration main merge input"

git switch -C "${TEST_ID}-feature-merge" "$ORIGINAL_HEAD" >/dev/null
commit_file "integration-feature-merge.txt" "feature before merge" "integration feature merge input"
git merge --no-ff "${TEST_ID}-main-merge" -m "integration merge main into feature" >/dev/null
assert_sandbox_head_matches
assert_sandbox_file_content "integration-main-merge.txt" "main merged into feature"
assert_sandbox_file_content "integration-feature-merge.txt" "feature before merge"

echo "Scenario: force-moved local branch removes files from old sandbox HEAD"
git switch -C "${TEST_ID}-force-move" "$ORIGINAL_HEAD" >/dev/null
commit_file "integration-force-move-old.txt" "old branch content" "integration old branch content"
assert_sandbox_file_content "integration-force-move-old.txt" "old branch content"

git reset --hard "$ORIGINAL_HEAD" >/dev/null
commit_file "integration-force-move-new.txt" "new branch content" "integration new branch content"
assert_sandbox_head_matches
assert_sandbox_file_missing "integration-force-move-old.txt"
assert_sandbox_file_content "integration-force-move-new.txt" "new branch content"

echo "Scenario: sandbox-created files pulled locally survive later local history movement"
git switch -C "${TEST_ID}-sandbox-created" "$ORIGINAL_HEAD" >/dev/null
"${RWX_CLI}" sandbox exec --id "$SANDBOX_RUN_ID" -- sh -c 'echo "created in sandbox" > integration-sandbox-created-survives.txt'
assert_local_file_content "integration-sandbox-created-survives.txt" "created in sandbox"

commit_file "integration-local-after-sandbox-created.txt" "local history moved after sandbox pull" "integration local move after sandbox pull"
assert_sandbox_head_matches
assert_sandbox_file_content "integration-sandbox-created-survives.txt" "created in sandbox"
assert_sandbox_file_content "integration-local-after-sandbox-created.txt" "local history moved after sandbox pull"

echo "Scenario: detached local HEAD is mirrored in sandbox"
git switch -C "${TEST_ID}-detached-source" "$ORIGINAL_HEAD" >/dev/null
commit_file "integration-detached-head.txt" "detached head content" "integration detached head"
detached_sha=$(git rev-parse HEAD)
git switch --detach "$detached_sha" >/dev/null
assert_sandbox_head_matches
assert_sandbox_file_content "integration-detached-head.txt" "detached head content"
sandbox_branch=$("${RWX_CLI}" sandbox exec --id "$SANDBOX_RUN_ID" -- sh -c 'git branch --show-current | sed "s/^/branch:/"' | awk -Fbranch: '/^branch:/{print $2; exit}')
if [ -n "$sandbox_branch" ]; then
  fail "sandbox branch should be detached but was ${sandbox_branch}"
fi

require_git_lfs

echo "Scenario: unpushed committed LFS objects fail after sandbox git push"
ensure_sandbox_git_lfs
git switch -C "${TEST_ID}-lfs-push" "$ORIGINAL_HEAD" >/dev/null
echo "Creating local committed LFS object"
commit_lfs_file "integration-lfs-push.bin" "committed lfs content" "integration committed lfs object"

echo "Running sandbox exec expected to fail because the committed LFS object was not pushed"
lfs_push_output_file=$(mktemp)
lfs_push_exit=0
run_and_capture_output "$lfs_push_output_file" "${RWX_CLI}" sandbox exec --id "$SANDBOX_RUN_ID" -- true || lfs_push_exit=$?
lfs_push_output=$(cat "$lfs_push_output_file")
rm -f "$lfs_push_output_file"
if [ "$lfs_push_exit" -eq 0 ]; then
  fail "sandbox exec succeeded with an unpushed committed LFS object"
fi
echo "$lfs_push_output" | grep -q "LFS file(s) changed locally and cannot be synced to the sandbox" || fail "missing LFS sync error for committed LFS object: ${lfs_push_output}"
echo "$lfs_push_output" | grep -q "integration-lfs-push.bin" || fail "missing committed LFS file path in error: ${lfs_push_output}"
echo "$lfs_push_output" | grep -q "To recover, push your changes and reset the sandbox." || fail "missing LFS sync recovery guidance: ${lfs_push_output}"

echo "Restarting sandbox after expected LFS sync failure"
start_git_state_sandbox

echo "Scenario: dirty LFS files are reported and skipped by patch sync"
git switch -C "${TEST_ID}-lfs-patch" "$ORIGINAL_HEAD" >/dev/null
git lfs track "integration-lfs-patch.bin" >/dev/null
printf '%s\n' "dirty lfs content" > integration-lfs-patch.bin

lfs_patch_output_file=$(mktemp)
lfs_patch_exit=0
run_and_capture_output "$lfs_patch_output_file" "${RWX_CLI}" sandbox exec --id "$SANDBOX_RUN_ID" -- sh -c 'test ! -e integration-lfs-patch.bin' || lfs_patch_exit=$?
lfs_patch_output=$(cat "$lfs_patch_output_file")
rm -f "$lfs_patch_output_file"
if [ "$lfs_patch_exit" -ne 0 ]; then
  fail "sandbox exec failed while checking dirty LFS patch skip: ${lfs_patch_output}"
fi
echo "$lfs_patch_output" | grep -q "Warning: 1 LFS file(s) changed locally and cannot be synced." || fail "missing dirty LFS warning: ${lfs_patch_output}"
if [ ! -f integration-lfs-patch.bin ]; then
  fail "dirty LFS file was removed locally after patch sync"
fi
git reset -- .gitattributes integration-lfs-patch.bin >/dev/null 2>&1 || true
rm -f .gitattributes integration-lfs-patch.bin

echo "Scenario: staged and unstaged local state keep their shape in sandbox"
git switch -C "${TEST_ID}-dirty-state" "$ORIGINAL_HEAD" >/dev/null
printf '%s\n' "staged state content" > integration-staged-state.txt
git add integration-staged-state.txt
printf '\n// integration unstaged state\n' >> go.mod

"${RWX_CLI}" sandbox exec --id "$SANDBOX_RUN_ID" -- sh -c '
  set -e
  git diff --cached --name-only | grep -qx integration-staged-state.txt
  git diff --name-only | grep -qx go.mod
  grep -q "integration unstaged state" go.mod
  test "$(cat integration-staged-state.txt)" = "staged state content"
'

git diff --cached --name-only | grep -qx integration-staged-state.txt || fail "local staged state was not preserved"
git diff --name-only | grep -qx go.mod || fail "local unstaged state was not preserved"

echo "PASS: sandbox git-state sync integration scenarios completed"
