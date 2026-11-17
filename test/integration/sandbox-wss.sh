#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/sandbox-helpers.sh"

export RWX_EXPERIMENTAL=true
ORIGINAL_HEAD=$(git rev-parse HEAD)

cleanup() {
  local status=$?
  set +e
  if [ -n "${SANDBOX_RUN_ID:-}" ]; then
    "${RWX_CLI}" sandbox background stop --transport wss --id "$SANDBOX_RUN_ID" --key wss-web >/dev/null 2>&1 || true
    stop_sandbox || status=1
  fi
  git reset --hard "$ORIGINAL_HEAD" >/dev/null || status=1
  rm -f integration-wss-committed.txt integration-wss-preview.txt integration-wss-pulled.txt || status=1
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# Record the run before waiting so a setup failure still reaches cleanup.
start_result=$("${RWX_CLI}" sandbox start "${SCRIPT_DIR}/definitions/sandbox-background.yml" \
  --json --init "ref=${COMMIT_SHA}")
SANDBOX_RUN_ID=$(echo "$start_result" | jq -er '.RunID')
echo "$start_result" | jq -er '.RunURL' > "$RWX_LINKS/Sandbox Run"
"${RWX_CLI}" sandbox start --id "$SANDBOX_RUN_ID" --wait >/dev/null

# A commit created after startup requires Git's OpenSSH connection, not just patch sync.
printf 'unpushed WSS commit\n' > integration-wss-committed.txt
git add integration-wss-committed.txt
git commit -m "integration WSS unpushed commit"
wss_head=$(git rev-parse HEAD)
printf 'initial WSS preview\n' > integration-wss-preview.txt

exec_status=0
# Expand Git and file checks in the remote shell, not the test runner.
# shellcheck disable=SC2016
exec_output=$("${RWX_CLI}" sandbox exec --transport wss --id "$SANDBOX_RUN_ID" -- sh -c '
  set -e
  test "$(git rev-parse HEAD)" = "$1"
  test "$(cat integration-wss-committed.txt)" = "unpushed WSS commit"
  test "$(cat integration-wss-preview.txt)" = "initial WSS preview"
  printf "pulled over WSS\n" > integration-wss-pulled.txt
  printf "WSS command output\n"
  exit 7
' sh "$wss_head") || exec_status=$?

command_output=$(printf '%s\n' "$exec_output" | awk 'NR==1')
if [ "$exec_status" -ne 7 ] || [ "$command_output" != "WSS command output" ]; then
  echo "WSS exec returned unexpected status or output (status: $exec_status): $exec_output"
  exit 1
fi
if [ "$(cat integration-wss-pulled.txt)" != "pulled over WSS" ]; then
  echo "WSS exec did not pull the remote file after a nonzero exit"
  exit 1
fi

echo "PASS: WSS execution, exit status, Git push, and reverse sync"

# A leaked ProxyCommand pipe must fail promptly rather than hang the workflow.
background_result=$(timeout 90 "${RWX_CLI}" sandbox background --transport wss \
  --id "$SANDBOX_RUN_ID" \
  --key wss-web \
  --port 3000 \
  --json \
  -- python3 -u -m http.server 3000 --bind 127.0.0.1)
preview_url=$(echo "$background_result" | jq -er '.URL')

if [[ "$preview_url" != http://127.0.0.1:* ]]; then
  echo "WSS background did not return a local forwarding URL: $preview_url"
  exit 1
fi

preview_content=$(curl --fail --silent --show-error --max-time 10 "$preview_url/integration-wss-preview.txt")
if [ "$preview_content" != "initial WSS preview" ]; then
  echo "WSS detached tunnel returned unexpected content: $preview_content"
  exit 1
fi

# Cross a heartbeat interval after the initiating CLI has exited.
sleep 45
preview_content=$(curl --fail --silent --show-error --max-time 10 "$preview_url/integration-wss-preview.txt")
if [ "$preview_content" != "initial WSS preview" ]; then
  echo "WSS detached tunnel did not survive idle: $preview_content"
  exit 1
fi

printf 'updated WSS preview\n' > integration-wss-preview.txt
"${RWX_CLI}" sandbox push --transport wss --id "$SANDBOX_RUN_ID" >/dev/null
preview_content=$(curl --fail --silent --show-error --max-time 10 "$preview_url/integration-wss-preview.txt")
if [ "$preview_content" != "updated WSS preview" ]; then
  echo "WSS push did not update forwarded content: $preview_content"
  exit 1
fi

"${RWX_CLI}" sandbox background stop --transport wss --id "$SANDBOX_RUN_ID" --key wss-web >/dev/null
if curl --fail --silent --max-time 2 "$preview_url/integration-wss-preview.txt" >/dev/null 2>&1; then
  echo "WSS detached tunnel remained reachable after stopping the background process"
  exit 1
fi

echo "PASS: WSS background process, detached forwarding, push, and tunnel cleanup"
