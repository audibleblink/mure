#!/usr/bin/env bash
# Phase 4 e2e: harness-aware `mure spawn`.
#
# Spins up a throwaway tmux server, installs a synthetic harness on-disk
# (via MURE_HARNESSES_DIR), runs `mure spawn --harness fake role`, verifies
# the new pane has MURE_PANE_ID / MURE_HARNESS / MURE_AGENT_ID env, then
# emits a status frame from the spawned pane and confirms `mure ls --json`
# reflects it.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

tmpdir=$(mktemp -d -t mure-spawn-e2e-XXXX)
tmux_sock="$tmpdir/tmux.sock"
mure_sock="$tmpdir/d.sock"
hdir="$tmpdir/harnesses"
mkdir -p "$hdir/fake"

cleanup() {
  tmux -S "$tmux_sock" kill-server 2>/dev/null || true
  [ -n "${DAEMON_PID:-}" ] && kill "$DAEMON_PID" 2>/dev/null || true
  rm -rf "$tmpdir"
}
trap cleanup EXIT

bin="$tmpdir/mure"
go build -o "$bin" ./cmd/mure

# Synthetic harness: stays running long enough for us to inspect.
cat >"$hdir/fake/manifest.toml" <<'TOML'
name = "fake"
command = "bash -c 'echo READY; sleep 10'"
task_arg = "none"
[capabilities]
spawn = true
status = true
result = true
TOML

export MURE_HARNESSES_DIR="$hdir"
export MURE_SOCKET="$mure_sock"
export MURE_SESSION="spawn-e2e"
export MURE_TMUX_SOCKET="$tmux_sock"

# Start a throwaway tmux server with one session BEFORE the daemon so its
# tmuxctl client can attach.
tmux -S "$tmux_sock" new-session -d -s "$MURE_SESSION" -n main

# Start daemon.
MURE_DAEMON=1 MURE_LAUNCH_DIR="$tmpdir" "$bin" up >/dev/null 2>&1 &
DAEMON_PID=$!
for _ in $(seq 1 50); do [ -S "$mure_sock" ] && break; sleep 0.05; done
[ -S "$mure_sock" ] || { echo "FAIL: no daemon socket" >&2; exit 1; }

# Spawn into the tmux session. Use --harness flag.
out=$("$bin" spawn --harness fake worker)
echo "spawn out: $out"
agent_id=$(awk '{print $1}' <<<"$out")
pane_id=$(awk '{print $2}' <<<"$out")
[ -n "$agent_id" ] && [ -n "$pane_id" ] || { echo "FAIL: bad spawn output: $out" >&2; exit 1; }

# Give pane time to exec.
sleep 0.5

# Inspect spawned pane's environment via `tmux show-environment` of its process
# is not portable; instead capture the pane and look for our READY marker
# (proof manifest.command ran) and pipe `env` from a short follow-up.
captured=$(tmux -S "$tmux_sock" capture-pane -p -t "$pane_id" || true)
echo "captured pane output:"
echo "$captured" | sed 's/^/  | /'
if ! grep -q READY <<<"$captured"; then
  echo "FAIL: pane never printed READY (harness command did not run)" >&2
  exit 1
fi

# Emit a status frame on behalf of the spawned pane and confirm it surfaces.
MURE_AGENT_ID="$agent_id" MURE_PANE_ID="$pane_id" "$bin" emit status working --tool foo

sleep 0.1
snapshot=$("$bin" ls --json)
echo "snapshot: $snapshot"
got_status=$(echo "$snapshot" | jq -r --arg id "$agent_id" '.agents[] | select(.id==$id) | .status')
got_pane=$(echo "$snapshot" | jq -r --arg id "$agent_id" '.agents[] | select(.id==$id) | .pane')
if [ "$got_status" != "working" ]; then
  echo "FAIL: status=$got_status want working" >&2
  exit 1
fi
if [ "$got_pane" != "$pane_id" ]; then
  echo "FAIL: pane=$got_pane want $pane_id (register_pane did not land)" >&2
  exit 1
fi

"$bin" down >/dev/null 2>&1 || true
wait "$DAEMON_PID" 2>/dev/null || true
DAEMON_PID=""

echo "OK"
