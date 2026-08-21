#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/sandbox-helpers.sh"

export EXPERIMENTAL=true

preview_name="web"
preview_port=3000
preview_file="integration-background-preview.txt"

cleanup() {
  if [ -n "${SANDBOX_RUN_ID:-}" ]; then
    "${RWX_CLI}" sandbox background stop --id "$SANDBOX_RUN_ID" --name "$preview_name" >/dev/null 2>&1 || true
    "${RWX_CLI}" sandbox stop --id "$SANDBOX_RUN_ID" >/dev/null 2>&1 || true
  fi
  rm -f "$preview_file"
}
trap cleanup EXIT

start_sandbox "${SCRIPT_DIR}/definitions/sandbox-background.yml"

printf 'initial preview\n' > "$preview_file"

start_result=$("${RWX_CLI}" sandbox background \
  --id "$SANDBOX_RUN_ID" \
  --name "$preview_name" \
  --port "$preview_port" \
  --json \
  -- python3 -u -m http.server "$preview_port" --bind 127.0.0.1)

preview_url=$(echo "$start_result" | jq -r '.URL')
start_pid=$(echo "$start_result" | jq -r '.PID')
start_time=$(echo "$start_result" | jq -r '.StartedAt')

if [ -z "$preview_url" ] || [ "$preview_url" = "null" ]; then
  echo "background start did not return a preview URL"
  exit 1
fi

initial_content=$(curl --fail --silent --show-error --max-time 10 "$preview_url/$preview_file")
if [ "$initial_content" != "initial preview" ]; then
  echo "unexpected initial preview content: $initial_content"
  exit 1
fi

printf 'updated preview\n' > "$preview_file"
"${RWX_CLI}" sandbox sync --id "$SANDBOX_RUN_ID" >/dev/null

updated_content=$(curl --fail --silent --show-error --max-time 10 "$preview_url/$preview_file")
if [ "$updated_content" != "updated preview" ]; then
  echo "sandbox sync did not update preview content: $updated_content"
  exit 1
fi

restart_result=$("${RWX_CLI}" sandbox background restart \
  --id "$SANDBOX_RUN_ID" \
  --name "$preview_name" \
  --json)

restart_url=$(echo "$restart_result" | jq -r '.URL')
restart_pid=$(echo "$restart_result" | jq -r '.PID')
restart_time=$(echo "$restart_result" | jq -r '.StartedAt')

if [ "$restart_url" != "$preview_url" ]; then
  echo "background restart changed preview URL (before: $preview_url, after: $restart_url)"
  exit 1
fi

if [ "$restart_pid" = "$start_pid" ] && [ "$restart_time" = "$start_time" ]; then
  echo "background restart did not replace the managed process"
  exit 1
fi

curl --fail --silent --show-error --max-time 10 "$restart_url/$preview_file" >/dev/null
logs_output=$("${RWX_CLI}" sandbox background logs --id "$SANDBOX_RUN_ID" --name "$preview_name")
if [[ "$logs_output" != *"$preview_file"* ]]; then
  echo "background logs did not include the preview request"
  echo "$logs_output"
  exit 1
fi

portless_result=$("${RWX_CLI}" sandbox background \
  --id "$SANDBOX_RUN_ID" \
  --name "$preview_name" \
  --json \
  -- sh -c 'while :; do sleep 60; done')

if [ "$(echo "$portless_result" | jq -r '.URL')" != "" ]; then
  echo "portless replacement unexpectedly returned a preview URL"
  exit 1
fi

if curl --fail --silent --max-time 2 "$preview_url/$preview_file" >/dev/null 2>&1; then
  echo "old preview tunnel remained reachable after portless replacement"
  exit 1
fi

"${RWX_CLI}" sandbox background stop --id "$SANDBOX_RUN_ID" --name "$preview_name" --json >/dev/null

echo "PASS: sandbox background process, sync, restart, logs, and tunnel cleanup"
