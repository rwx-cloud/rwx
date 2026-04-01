#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/sandbox-helpers.sh"

start_sandbox
trap stop_sandbox EXIT

bash_c_output=$("${RWX_CLI}" sandbox exec --no-sync -- bash -c "echo hello world" | awk 'NR==1')
if [ "$bash_c_output" != "hello world" ]; then
  echo "bash -c shell quoting failed (expected 'hello world', got '$bash_c_output')"
  exit 1
fi
