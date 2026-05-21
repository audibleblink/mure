#!/usr/bin/env bash
# Phase 3 e2e: start a real mure daemon, run `mure emit status working --tool foo`,
# then assert via `mure ls --json` that the status frame surfaced in the roster.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

tmpdir=$(mktemp -d -t mure-emit-e2e-XXXX)
trap 'rm -rf "$tmpdir"; [ -n "${DAEMON_PID:-}" ] && kill "$DAEMON_PID" 2>/dev/null || true' EXIT

bin="$tmpdir/mure"
go build -o "$bin" ./cmd/mure

sock="$tmpdir/d.sock"
export MURE_SOCKET="$sock"
export MURE_SESSION="emit-e2e"

# Run the daemon in-foreground (MURE_DAEMON=1 short-circuits the fork in
# cmd/mure/up.go), so the e2e script controls the pid for cleanup.
MURE_DAEMON=1 MURE_LAUNCH_DIR="$tmpdir" "$bin" up >/dev/null 2>&1 &
DAEMON_PID=$!

for _ in $(seq 1 50); do
  [ -S "$sock" ] && break
  sleep 0.05
done
if [ ! -S "$sock" ]; then
  echo "FAIL: daemon never created socket $sock" >&2
  exit 1
fi

export MURE_AGENT_ID="agent-e2e"
"$bin" emit status working --tool foo

# Give the daemon a tick to apply the frame.
sleep 0.1

snapshot=$("$bin" ls --json)
echo "snapshot: $snapshot"

got_status=$(echo "$snapshot" | jq -r --arg id "$MURE_AGENT_ID" '.agents[] | select(.id==$id) | .status')
if [ "$got_status" != "working" ]; then
  echo "FAIL: expected status=working for $MURE_AGENT_ID, got $got_status" >&2
  exit 1
fi

"$bin" down >/dev/null 2>&1 || true
wait "$DAEMON_PID" 2>/dev/null || true
DAEMON_PID=""

echo "OK"
