#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/sandbox-helpers.sh"

# Reproduces the failure from https://gist.github.com/dan-manges/4f69dbe13bc6087d2f5eeed7794c1911:
# local HEAD is an unpushed merge commit, the sandbox workspace is a depth-1
# clone (git/clone's default), and the pre-exec sync push cannot resolve history
# the sandbox has never had. `sandbox start` succeeds; every `sandbox exec`
# fails before the payload runs.
#
# Unlike sandbox-shallow-clone-sync.sh, which covers a shallow *local* clone
# pushing to a full-history sandbox, this covers the inverse: a full local clone
# pushing to a shallow *sandbox*.
#
# The definition also makes the sandbox's origin unreachable. That is load
# bearing, not incidental: without it the sandbox self-heals via an anonymous
# `git fetch origin` and this scenario passes for a reason that does not hold for
# the private repos hitting it. See the comment in the definition.
SANDBOX_CONFIG="${SCRIPT_DIR}/definitions/sandbox-shallow-depth.yml"
SANDBOX_RUN_ID=""
BRANCH="unpushed-merge-sync-$$"
MARKER="unpushed-merge-marker.txt"
MARKER_CONTENT="unpushed merge marker"
ORIGINAL_HEAD=$(git rev-parse HEAD)

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

cleanup() {
  set +e
  if [ -n "$SANDBOX_RUN_ID" ]; then
    "${RWX_CLI}" sandbox stop --id "$SANDBOX_RUN_ID" >/dev/null 2>&1
  fi
  rm -f "$MARKER"
  git reset --hard "$ORIGINAL_HEAD" >/dev/null 2>&1
  git checkout --detach "$ORIGINAL_HEAD" >/dev/null 2>&1
  git branch -D "$BRANCH" >/dev/null 2>&1
}
trap cleanup EXIT

rm -rf .rwx/sandboxes

# The scenario under test is sandbox-side shallowness, so make sure local
# shallowness cannot be what fails the push. That path is already covered by
# sandbox-shallow-clone-sync.sh.
export GIT_TERMINAL_PROMPT=0
if [ "$(git rev-parse --is-shallow-repository)" = "true" ]; then
  echo "Unshallowing the local checkout so local history is not the limiting factor"
  # This checkout inherits git/clone's credential helper, which authenticates
  # with an empty password now that GITHUB_TOKEN is out of scope. An empty
  # credential.helper resets the helper list so the fetch is anonymous.
  git -c credential.helper= fetch --quiet --unshallow origin \
    || fail "could not unshallow the local checkout"
fi
if [ "$(git rev-parse --is-shallow-repository)" != "false" ]; then
  fail "local checkout is still shallow; this scenario requires full local history"
fi

if [ "$(git rev-list --count "$ORIGINAL_HEAD")" -lt 6 ]; then
  fail "this scenario requires at least 6 commits of local history"
fi

# Provision the sandbox at exactly the local HEAD, depth 1. The sandbox holds
# that one commit and none of its ancestors.
sandbox_result=$("${RWX_CLI}" sandbox start "$SANDBOX_CONFIG" --json \
  --init ref=main --init "clone-ref=${ORIGINAL_HEAD}" --init "cli=${COMMIT_SHA}" --wait)
SANDBOX_RUN_ID=$(echo "$sandbox_result" | jq -r ".RunID")

sandbox_url=$(echo "$sandbox_result" | jq -r ".RunURL // empty")
if [ -n "$sandbox_url" ]; then
  echo "Sandbox URL: ${sandbox_url}"
  echo "$sandbox_url" > "$RWX_LINKS/Sandbox Run"
fi
if [ -z "$SANDBOX_RUN_ID" ] || [ "$SANDBOX_RUN_ID" = "null" ]; then
  echo "$sandbox_result"
  fail "sandbox start did not return a run id"
fi

# Control: local HEAD already matches the sandbox's clone sha, so the sync has
# no commits to push and must succeed. This also pins the precondition: the
# sandbox workspace is a depth-1 clone.
echo "Scenario: control exec against a depth-1 sandbox at local HEAD"
control_cmd='set -e; test "$(/usr/bin/git rev-parse --is-shallow-repository)" = true; test "$(/usr/bin/git rev-parse HEAD)" = '"$ORIGINAL_HEAD"
"${RWX_CLI}" sandbox exec --id "$SANDBOX_RUN_ID" -- sh -c "$control_cmd" \
  || fail "control exec failed; the sandbox is not in the expected depth-1 state at ${ORIGINAL_HEAD}"

# Build an unpushed merge commit whose second parent the sandbox does not have.
# Using the current tree keeps the working directory untouched. The second
# parent is an ancestor of HEAD, so it exists locally but was pruned from the
# sandbox's depth-1 clone.
SECOND_PARENT=$(git rev-parse "${ORIGINAL_HEAD}~5")
MERGE_COMMIT=$(git -c user.email="sandbox-integration@example.com" -c user.name="Sandbox Integration" \
  commit-tree "${ORIGINAL_HEAD}^{tree}" -p "$ORIGINAL_HEAD" -p "$SECOND_PARENT" -m "unpushed merge commit")
git checkout --quiet -B "$BRANCH" "$MERGE_COMMIT"

if [ "$(git rev-parse HEAD)" != "$MERGE_COMMIT" ]; then
  fail "expected local HEAD to be the unpushed merge commit ${MERGE_COMMIT}"
fi

printf '%s\n' "$MARKER_CONTENT" > "$MARKER"

echo "Scenario: sandbox exec syncs an unpushed merge commit into a depth-1 sandbox"
output_file=$(mktemp)
exit_code=0
"${RWX_CLI}" sandbox exec --id "$SANDBOX_RUN_ID" -- sh -c \
  'set -e; test "$(/usr/bin/git rev-parse HEAD)" = '"$MERGE_COMMIT"'; test "$(cat '"$MARKER"')" = "'"$MARKER_CONTENT"'"' \
  >"$output_file" 2>&1 || exit_code=$?
output=$(cat "$output_file")
rm -f "$output_file"
echo "$output"

if [ "$exit_code" -ne 0 ]; then
  echo "$output" | grep -qi "unresolved deltas\|missing necessary objects\|unpacker error" \
    && fail "sync push could not resolve history missing from the depth-1 sandbox (exit ${exit_code})"
  fail "sandbox exec failed with an unexpected error (exit ${exit_code})"
fi

echo "PASS: sandbox exec synced an unpushed merge commit into a depth-1 sandbox"
