#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/sandbox-helpers.sh"

start_sandbox
trap stop_sandbox EXIT

git commit --allow-empty -m "unpushed local commit"
echo "uncommitted edit" > uncommitted-test.txt
uncommitted_sha=$(sha1sum uncommitted-test.txt | awk '{print $1}')

"${RWX_CLI}" sandbox exec -- echo "exercising sandbox with unpushed commits"

post_exec_sha=$(sha1sum uncommitted-test.txt | awk '{print $1}')
if [ "$uncommitted_sha" != "$post_exec_sha" ]; then
  echo "uncommitted-test.txt was lost during sandbox exec with unpushed commits (expected $uncommitted_sha, got $post_exec_sha)"
  exit 1
fi
