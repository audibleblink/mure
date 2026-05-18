#!/usr/bin/env bash
# Manual smoke test: start tmux, bring up mure, spawn a dummy pane, ls.
# Requires tmux >= 3.2 on PATH.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MURE="$ROOT/bin/mure"

make -C "$ROOT" build >/dev/null

TMUX_TMPDIR=$(mktemp -d)
export TMUX_TMPDIR
SESSION="mure-smoke-$$"
SOCK_TMUX="mure-smoke"

cleanup() {
    tmux -L "$SOCK_TMUX" kill-server 2>/dev/null || true
    rm -rf "$TMUX_TMPDIR"
}
trap cleanup EXIT

tmux -L "$SOCK_TMUX" new-session -d -s "$SESSION" 'sleep 30'

RUN_DIR="$(MURE_SESSION=$SESSION "$MURE" up 2>&1 || true)"
echo "$RUN_DIR"

# Resolve socket path: same convention as daemon paths.
if [[ "$(uname)" == "Darwin" ]]; then
    SOCKET="$HOME/Library/Caches/mure/$SESSION/daemon.sock"
else
    SOCKET="${XDG_RUNTIME_DIR:-/tmp/mure-$(id -u)}/mure/$SESSION/daemon.sock"
fi

# Wait for socket
for _ in 1 2 3 4 5 6 7 8 9 10; do
    [[ -S "$SOCKET" ]] && break
    sleep 0.2
done
[[ -S "$SOCKET" ]] || { echo "socket never appeared at $SOCKET" >&2; exit 1; }

export MURE_SOCKET="$SOCKET"
"$MURE" ls
"$MURE" down
echo "smoke ok"
