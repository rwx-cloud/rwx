#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/sandbox-helpers.sh"

start_sandbox
trap stop_sandbox EXIT

echo "revert-test content" > revert-test.txt
"${RWX_CLI}" sandbox exec -- cat revert-test.txt > /dev/null
rm -f revert-test.txt

"${RWX_CLI}" sandbox exec -- echo "exec after local revert"

if [ -f revert-test.txt ]; then
  echo "revert-test.txt was pulled back from sandbox after being reverted locally"
  exit 1
fi
