#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/sandbox-helpers.sh"

start_sandbox
trap stop_sandbox EXIT

# First exec: reconnection hint should NOT appear
first_stderr=$("${RWX_CLI}" sandbox exec -- sh -c 'echo first' 2>&1 >/dev/null) || true
if echo "$first_stderr" | grep -q "Reconnecting to existing sandbox"; then
  echo "ERROR: Unexpected reconnection hint on first exec"
  echo "stderr was: $first_stderr"
  exit 1
fi

# Second exec: reconnection hint SHOULD appear
second_stderr=$("${RWX_CLI}" sandbox exec -- sh -c 'echo second' 2>&1 >/dev/null) || true
if ! echo "$second_stderr" | grep -q "Reconnecting to existing sandbox"; then
  echo "ERROR: Expected reconnection hint on second exec but did not find it"
  echo "stderr was: $second_stderr"
  exit 1
fi

# Third exec: hint should NOT appear again (only shown once)
third_stderr=$("${RWX_CLI}" sandbox exec -- sh -c 'echo third' 2>&1 >/dev/null) || true
if echo "$third_stderr" | grep -q "Reconnecting to existing sandbox"; then
  echo "ERROR: Unexpected reconnection hint on third exec (should only appear once)"
  exit 1
fi
