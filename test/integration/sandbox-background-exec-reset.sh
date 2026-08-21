#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/sandbox-helpers.sh"

export EXPERIMENTAL=true

start_sandbox "${SCRIPT_DIR}/definitions/sandbox-background.yml"
trap stop_sandbox EXIT

background_result=$("${RWX_CLI}" sandbox background \
  --id "$SANDBOX_RUN_ID" \
  --name reset-preview \
  --port 3000 \
  --json \
  -- python3 -u -m http.server 3000 --bind 127.0.0.1)
preview_url=$(echo "$background_result" | jq -r '.URL')

curl --fail --silent --show-error --max-time 10 "$preview_url" >/dev/null

"${RWX_CLI}" sandbox exec \
  --reset \
  --init ref=main \
  --init "cli=${COMMIT_SHA}" \
  "${SCRIPT_DIR}/definitions/sandbox-background.yml" \
  -- true

if curl --fail --silent --max-time 2 "$preview_url" >/dev/null 2>&1; then
  echo "ERROR: Preview tunnel remained reachable after exec --reset"
  exit 1
fi

echo "PASS: sandbox exec --reset closed the old background tunnel"
