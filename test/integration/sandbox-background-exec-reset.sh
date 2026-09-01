#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/sandbox-helpers.sh"

export RWX_EXPERIMENTAL=true

start_sandbox "${SCRIPT_DIR}/definitions/sandbox-background.yml"
trap stop_sandbox EXIT

background_result=$("${RWX_CLI}" sandbox background \
  --id "$SANDBOX_RUN_ID" \
  --key reset-preview \
  --port 3000 \
  --json \
  -- python3 -u -m http.server 3000 --bind 127.0.0.1)
preview_url=$(echo "$background_result" | jq -r '.URL')

curl --fail --silent --show-error --max-time 10 "$preview_url" >/dev/null

exec_result=$("${RWX_CLI}" sandbox exec \
  --json \
  --reset \
  --init ref=main \
  --init "cli=${COMMIT_SHA}" \
  "${SCRIPT_DIR}/definitions/sandbox-background.yml" \
  -- true)
SANDBOX_RUN_ID=$(echo "$exec_result" | jq -r '.RunID // empty')
if [ -z "$SANDBOX_RUN_ID" ]; then
  echo "exec --reset did not return a run ID"
  echo "$exec_result"
  exit 1
fi

if curl --fail --silent --max-time 2 "$preview_url" >/dev/null 2>&1; then
  echo "ERROR: Preview tunnel remained reachable after exec --reset"
  exit 1
fi

echo "PASS: sandbox exec --reset closed the old background tunnel"
