#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/sandbox-helpers.sh"

export RWX_EXPERIMENTAL=true

configured_name="configured-web"
configured_log_marker=$'\tstdout\tconfigured-background-process-started'

start_sandbox "${SCRIPT_DIR}/definitions/sandbox-configured-background.yml"
trap stop_sandbox EXIT

"${RWX_CLI}" sandbox exec --id "$SANDBOX_RUN_ID" -- \
  python3 -c 'import urllib.request; assert urllib.request.urlopen("http://127.0.0.1:3001", timeout=10).status == 200'

configured_logs=""
for _ in {1..30}; do
  configured_logs=$("${RWX_CLI}" sandbox background logs \
    --id "$SANDBOX_RUN_ID" \
    --name "$configured_name") || true
  if [[ "$configured_logs" == *"$configured_log_marker"* ]]; then
    break
  fi
  sleep 1
done

if [[ "$configured_logs" != *"$configured_log_marker"* ]]; then
  echo "configured background logs did not include the startup marker"
  echo "$configured_logs"
  exit 1
fi

configured_restart_result=$("${RWX_CLI}" sandbox background restart \
  --id "$SANDBOX_RUN_ID" \
  --name "$configured_name" \
  --json)

if [ "$(echo "$configured_restart_result" | jq -r '.Status')" != "running" ]; then
  echo "configured background process was not running after restart"
  echo "$configured_restart_result"
  exit 1
fi

"${RWX_CLI}" sandbox exec --id "$SANDBOX_RUN_ID" -- \
  python3 -c 'import urllib.request; assert urllib.request.urlopen("http://127.0.0.1:3001", timeout=10).status == 200'

configured_start_count=0
for _ in {1..30}; do
  configured_logs=$("${RWX_CLI}" sandbox background logs \
    --id "$SANDBOX_RUN_ID" \
    --name "$configured_name") || true
  configured_start_count=$(grep -c "$configured_log_marker" <<< "$configured_logs" || true)
  if [ "$configured_start_count" -ge 2 ]; then
    break
  fi
  sleep 1
done

if [ "$configured_start_count" -lt 2 ]; then
  echo "configured background logs did not include both process generations after restart"
  echo "$configured_logs"
  exit 1
fi

echo "PASS: configured sandbox background process interaction"
