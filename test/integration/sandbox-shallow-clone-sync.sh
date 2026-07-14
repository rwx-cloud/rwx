#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/sandbox-helpers.sh"

SANDBOX_CONFIG="${SCRIPT_DIR}/definitions/sandbox.yml"
SANDBOX_RUN_ID=""
WORKDIR=""

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

cleanup() {
  set +e
  if [ -n "$SANDBOX_RUN_ID" ]; then
    "${RWX_CLI}" sandbox stop --id "$SANDBOX_RUN_ID" >/dev/null 2>&1
  fi
  if [ -n "$WORKDIR" ]; then
    rm -rf "$WORKDIR"
  fi
}
trap cleanup EXIT

sandbox_result=$("${RWX_CLI}" sandbox start "$SANDBOX_CONFIG" --json --init ref=main --init "cli=${COMMIT_SHA}" --wait)
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

# Build a shallow local clone whose history the sandbox has never seen, so the
# pre-exec sync must push a commit rooted at a shallow boundary the sandbox
# lacks. git-receive-pack refuses to record the new graft ("shallow update not
# allowed"), reproducing the failure from RFC 201.
WORKDIR=$(mktemp -d)
ORIGIN="${WORKDIR}/origin"
SHALLOW="${WORKDIR}/shallow"

git init -q "$ORIGIN"
for i in 1 2 3; do
  printf '%s\n' "origin commit $i" > "${ORIGIN}/f"
  git -C "$ORIGIN" add f
  git -C "$ORIGIN" -c user.email="shallow-integration@example.com" -c user.name="Shallow Integration" commit -qm "origin commit $i"
done

git clone -q --depth 1 "file://${ORIGIN}" "$SHALLOW"
if [ "$(git -C "$SHALLOW" rev-parse --is-shallow-repository)" != "true" ]; then
  fail "expected the local clone to be shallow"
fi

printf '%s\n' "local shallow change" > "${SHALLOW}/f"
git -C "$SHALLOW" add f
git -C "$SHALLOW" -c user.email="shallow-integration@example.com" -c user.name="Shallow Integration" commit -qm "local commit on shallow boundary"

echo "Scenario: sandbox exec from a shallow local clone is coached, not left with a raw git error"
output_file=$(mktemp)
exit_code=0
( cd "$SHALLOW" && "${RWX_CLI}" sandbox exec --id "$SANDBOX_RUN_ID" -- true ) >"$output_file" 2>&1 || exit_code=$?
output=$(cat "$output_file")
rm -f "$output_file"
echo "$output"

if [ "$exit_code" -eq 0 ]; then
  fail "sandbox exec unexpectedly succeeded pushing from a shallow local clone"
fi

echo "$output" | grep -qi "shallow" || fail "sync failure was not about a shallow clone: ${output}"
echo "$output" | grep -q "git fetch --unshallow" || fail "missing shallow-clone recovery guidance (git fetch --unshallow): ${output}"
echo "$output" | grep -q "fetch-full-depth: true" || fail "missing shallow-clone recovery guidance (fetch-full-depth: true): ${output}"
echo "$output" | grep -q "sandbox exec --reset" || fail "missing shallow-clone recovery guidance (sandbox exec --reset): ${output}"

echo "PASS: sandbox shallow-clone sync coaching integration scenario completed"
