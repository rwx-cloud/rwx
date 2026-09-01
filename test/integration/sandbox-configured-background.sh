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

if missing_tunnel_output=$("${RWX_CLI}" sandbox tunnel \
  --id "$SANDBOX_RUN_ID" \
  --key missing-web \
  --port 3001 2>&1); then
  echo "tunneling to a missing background process unexpectedly succeeded"
  exit 1
fi

if [[ "$missing_tunnel_output" != *"rwx sandbox background --key missing-web -- <command>"* ]]; then
  echo "missing background process error did not explain how to start one"
  echo "$missing_tunnel_output"
  exit 1
fi

configured_tunnel_result=$("${RWX_CLI}" sandbox tunnel \
  --id "$SANDBOX_RUN_ID" \
  --key "$configured_name" \
  --port 3001 \
  --json)
configured_tunnel_url=$(echo "$configured_tunnel_result" | jq -r '.URL')

if [ -z "$configured_tunnel_url" ] || [ "$configured_tunnel_url" = "null" ]; then
  echo "configured background tunnel did not return a URL"
  echo "$configured_tunnel_result"
  exit 1
fi

curl --fail --silent --show-error --max-time 10 "$configured_tunnel_url" >/dev/null

reattached_tunnel_url=$("${RWX_CLI}" sandbox tunnel \
  --id "$SANDBOX_RUN_ID" \
  --key "$configured_name" \
  --port 3001 \
  --json | jq -r '.URL')

if [ "$reattached_tunnel_url" != "$configured_tunnel_url" ]; then
  echo "reopening configured background tunnel changed its URL"
  echo "before: $configured_tunnel_url"
  echo "after:  $reattached_tunnel_url"
  exit 1
fi

configured_logs=""
for _ in {1..30}; do
  configured_logs=$("${RWX_CLI}" sandbox background logs \
    --id "$SANDBOX_RUN_ID" \
    --key "$configured_name") || true
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

if [ "$(grep -c "$configured_log_marker" <<< "$configured_logs" || true)" -ne 1 ]; then
  echo "opening a tunnel unexpectedly restarted the configured background process"
  echo "$configured_logs"
  exit 1
fi

configured_restart_result=$("${RWX_CLI}" sandbox background restart \
  --id "$SANDBOX_RUN_ID" \
  --key "$configured_name" \
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
    --key "$configured_name") || true
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
