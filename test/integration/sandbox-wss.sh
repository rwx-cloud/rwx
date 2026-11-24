#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/sandbox-helpers.sh"

ORIGINAL_HEAD=$(git rev-parse HEAD)
test_home=$(mktemp -d)
firewall_chain="RWX_WSS_$$"
firewall=iptables
firewall_created=false

# Isolate the CLI's saved preference without losing the test runner's Git identity.
export GIT_CONFIG_GLOBAL="${GIT_CONFIG_GLOBAL:-$HOME/.gitconfig}"
export HOME="$test_home"

cleanup() {
  local status=$?
  set +e
  if [ "$firewall_created" = true ]; then
    if sudo -n "$firewall" -w -C OUTPUT -j "$firewall_chain" 2>/dev/null; then
      sudo -n "$firewall" -w -D OUTPUT -j "$firewall_chain" || status=1
    fi
    sudo -n "$firewall" -w -F "$firewall_chain" || status=1
    sudo -n "$firewall" -w -X "$firewall_chain" || status=1
  fi
  if [ -n "${SANDBOX_RUN_ID:-}" ]; then
    "${RWX_CLI}" sandbox background stop --transport wss --id "$SANDBOX_RUN_ID" --key wss-web >/dev/null 2>&1 || true
    stop_sandbox || status=1
  fi
  git reset --hard "$ORIGINAL_HEAD" >/dev/null || status=1
  rm -f integration-wss-committed.txt integration-wss-preview.txt integration-wss-pulled.txt || status=1
  rm -rf "$test_home" || status=1
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# Record the run before waiting so a setup failure still reaches cleanup.
start_result=$("${RWX_CLI}" sandbox start "${SCRIPT_DIR}/definitions/sandbox-background.yml" \
  --json --init "ref=${COMMIT_SHA}")
SANDBOX_RUN_ID=$(echo "$start_result" | jq -er '.RunID')
echo "$start_result" | jq -er '.RunURL' > "$RWX_LINKS/Sandbox Run"
"${RWX_CLI}" sandbox start --id "$SANDBOX_RUN_ID" --wait >/dev/null

# Keep the token out of command arguments and discard the response's key material.
ssh_address=$(printf 'Authorization: Bearer %s\n' "$RWX_ACCESS_TOKEN" |
  curl --fail --silent --show-error --header @- \
    "https://${MINT_HOST:-${RWX_HOST:-cloud.rwx.com}}/mint/api/sandbox_connection_info?sandbox_key=${SANDBOX_RUN_ID}" |
  jq -er '.address')
ssh_host=${ssh_address%:*}
ssh_host=${ssh_host#[}
ssh_host=${ssh_host%]}
ssh_port=${ssh_address##*:}
if [[ "$ssh_host" == *:* ]]; then
  firewall=ip6tables
fi

# Reject only this sandbox's direct SSH endpoint; HTTPS/WSS stays reachable.
sudo -n "$firewall" -w -N "$firewall_chain"
firewall_created=true
sudo -n "$firewall" -w -A "$firewall_chain" -d "$ssh_host" -p tcp --dport "$ssh_port" \
  -j REJECT --reject-with tcp-reset
sudo -n "$firewall" -w -I OUTPUT -j "$firewall_chain"

# A commit created after startup requires Git's OpenSSH connection, not just patch sync.
printf 'unpushed WSS commit\n' > integration-wss-committed.txt
git add integration-wss-committed.txt
git commit -m "integration WSS unpushed commit"
wss_head=$(git rev-parse HEAD)
printf 'initial WSS preview\n' > integration-wss-preview.txt

exec_status=0
# Expand Git and file checks in the remote shell, not the test runner.
# shellcheck disable=SC2016
exec_output=$("${RWX_CLI}" sandbox exec --id "$SANDBOX_RUN_ID" -- sh -c '
  set -e
  test "$(git rev-parse HEAD)" = "$1"
  test "$(cat integration-wss-committed.txt)" = "unpushed WSS commit"
  test "$(cat integration-wss-preview.txt)" = "initial WSS preview"
  printf "pulled over WSS\n" > integration-wss-pulled.txt
  printf "WSS command output\n"
  exit 7
' sh "$wss_head") || exec_status=$?

command_output=$(printf '%s\n' "$exec_output" | awk 'NR==1')
if [ "$exec_status" -ne 7 ] || [ "$command_output" != "WSS command output" ]; then
  echo "WSS exec returned unexpected status or output (status: $exec_status): $exec_output"
  exit 1
fi
if [ "$(cat integration-wss-pulled.txt)" != "pulled over WSS" ]; then
  echo "WSS exec did not pull the remote file after a nonzero exit"
  exit 1
fi

if [ "$(cat "$HOME/.config/rwx/connection-transport")" != "wss" ]; then
  echo "Successful fallback did not save the WSS preference"
  exit 1
fi
blocked_ssh_packets=$(sudo -n "$firewall" -w -L "$firewall_chain" -n -v -x | awk '$3 == "REJECT" {print $1}')
if [ "$blocked_ssh_packets" -eq 0 ]; then
  echo "Auto transport did not attempt direct SSH before using WSS"
  exit 1
fi

echo "PASS: SSH-to-WSS fallback, saved preference, execution, Git push, and reverse sync"

# A leaked ProxyCommand pipe must fail promptly rather than hang the workflow.
background_result=$(timeout 90 "${RWX_CLI}" sandbox background \
  --id "$SANDBOX_RUN_ID" \
  --key wss-web \
  --port 3000 \
  --json \
  -- python3 -u -m http.server 3000 --bind 127.0.0.1)
preview_url=$(echo "$background_result" | jq -er '.URL')

if [[ "$preview_url" != http://127.0.0.1:* ]]; then
  echo "WSS background did not return a local forwarding URL: $preview_url"
  exit 1
fi

preview_content=$(curl --fail --silent --show-error --max-time 10 "$preview_url/integration-wss-preview.txt")
if [ "$preview_content" != "initial WSS preview" ]; then
  echo "WSS detached tunnel returned unexpected content: $preview_content"
  exit 1
fi

# Cross a heartbeat interval after the initiating CLI has exited.
sleep 45
preview_content=$(curl --fail --silent --show-error --max-time 10 "$preview_url/integration-wss-preview.txt")
if [ "$preview_content" != "initial WSS preview" ]; then
  echo "WSS detached tunnel did not survive idle: $preview_content"
  exit 1
fi

printf 'updated WSS preview\n' > integration-wss-preview.txt
"${RWX_CLI}" sandbox push --id "$SANDBOX_RUN_ID" >/dev/null
preview_content=$(curl --fail --silent --show-error --max-time 10 "$preview_url/integration-wss-preview.txt")
if [ "$preview_content" != "updated WSS preview" ]; then
  echo "WSS push did not update forwarded content: $preview_content"
  exit 1
fi

"${RWX_CLI}" sandbox background stop --id "$SANDBOX_RUN_ID" --key wss-web >/dev/null
if curl --fail --silent --max-time 2 "$preview_url/integration-wss-preview.txt" >/dev/null 2>&1; then
  echo "WSS detached tunnel remained reachable after stopping the background process"
  exit 1
fi

final_ssh_packets=$(sudo -n "$firewall" -w -L "$firewall_chain" -n -v -x | awk '$3 == "REJECT" {print $1}')
if [ "$final_ssh_packets" -ne "$blocked_ssh_packets" ]; then
  echo "Later commands retried direct SSH despite the saved WSS preference"
  exit 1
fi

echo "PASS: remembered WSS without SSH retries, detached forwarding, push, and tunnel cleanup"
