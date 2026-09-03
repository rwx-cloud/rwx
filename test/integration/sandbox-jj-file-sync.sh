#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/sandbox-helpers.sh"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

command -v jj > /dev/null 2>&1 || fail "jj is not installed"

export JJ_USER="Testing"
export JJ_EMAIL="git@example.com"

# Colocate the existing git checkout so jj drives the same working copy, which
# selects the jj backend.
#
# Colocating puts @ one commit above the imported HEAD, so the CLI keys the
# sandbox session by the nearest bookmark below @, or by @'s change id when the
# checkout carries none. Either key has to survive the edits below, since jj
# rewrites @ on every snapshot.
jj --no-pager --color=never --quiet git init --colocate > /dev/null

jj_head() {
  jj --no-pager --color=never --quiet log -r '@' --no-graph -T 'commit_id'
}

start_sandbox
trap stop_sandbox EXIT

echo "new file from jj" > jj-new-file.txt
echo "# Change from jj" >> go.mod

new_file_sha=$(sha1sum jj-new-file.txt | awk '{print $1}')
changed_file_sha=$(sha1sum go.mod | awk '{print $1}')

sandbox_new_file_sha=$("${RWX_CLI}" sandbox exec -- sha1sum jj-new-file.txt | awk 'NR==1{print $1}')
if [ "$new_file_sha" != "$sandbox_new_file_sha" ]; then
  fail "jj-new-file.txt content mismatch in sandbox (local: $new_file_sha, sandbox: $sandbox_new_file_sha)"
fi

sandbox_changed_file_sha=$("${RWX_CLI}" sandbox exec -- sha1sum go.mod | awk 'NR==1{print $1}')
if [ "$changed_file_sha" != "$sandbox_changed_file_sha" ]; then
  fail "go.mod content mismatch in sandbox (local: $changed_file_sha, sandbox: $sandbox_changed_file_sha)"
fi

post_new_file_sha=$(sha1sum jj-new-file.txt | awk '{print $1}')
if [ "$new_file_sha" != "$post_new_file_sha" ]; then
  fail "jj-new-file.txt was modified during sandbox exec (expected $new_file_sha, got $post_new_file_sha)"
fi

post_changed_file_sha=$(sha1sum go.mod | awk '{print $1}')
if [ "$changed_file_sha" != "$post_changed_file_sha" ]; then
  fail "go.mod was modified during sandbox exec (expected $changed_file_sha, got $post_changed_file_sha)"
fi

# jj snapshots the working copy into @, so the sandbox is synced by pushing that
# commit rather than by applying a dirty patch on top of it. A second exec after
# a further edit must still land, which is what catches a stale snapshot.
first_head=$(jj_head)

echo "second change from jj" >> jj-new-file.txt
second_file_sha=$(sha1sum jj-new-file.txt | awk '{print $1}')

sandbox_second_file_sha=$("${RWX_CLI}" sandbox exec -- sha1sum jj-new-file.txt | awk 'NR==1{print $1}')
if [ "$second_file_sha" != "$sandbox_second_file_sha" ]; then
  fail "second jj-new-file.txt change did not sync (local: $second_file_sha, sandbox: $sandbox_second_file_sha)"
fi

second_head=$(jj_head)
if [ "$first_head" = "$second_head" ]; then
  fail "expected @ to advance after editing the working copy (still $first_head)"
fi

# The sandbox must contain the working-copy commit itself, since that is what
# the jj backend pushes in place of a dirty patch.
sandbox_has_head=$("${RWX_CLI}" sandbox exec -- git cat-file -t "$second_head" 2>/dev/null | awk 'NR==1{print $1}' || true)
if [ "$sandbox_has_head" != "commit" ]; then
  fail "sandbox does not contain the jj working-copy commit $second_head (got '$sandbox_has_head')"
fi
