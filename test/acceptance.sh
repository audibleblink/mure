#!/usr/bin/env bash
# Phase 5 acceptance: PRD §14 items as one runnable suite.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

bin="$ROOT/bin/mure"
[ -x "$bin" ] || go build -o "$bin" ./cmd/mure

pass() { printf '  ok  %s\n' "$1"; }
fail() { printf '  FAIL %s\n' "$1" >&2; exit 1; }

# 1. Manifests validate — `go test` covers TestEmbeddedManifestsAllDecode.
go test ./internal/harnesses/... >/dev/null
pass "manifests validate"

# 2. List shows exactly pi, claude, opencode.
got=$("$bin" integration list | awk 'NR>1 {print $1}' | sort | tr '\n' ' ')
want="claude opencode pi "
[ "$got" = "$want" ] || fail "integration list got=[$got] want=[$want]"
pass "integration list shows pi, claude, opencode"

# 3. install pi → install pi (no-op) → uninstall pi leaves HOME clean.
HOME_DIR="$(mktemp -d)"
trap 'rm -rf "$HOME_DIR"' EXIT
HOME="$HOME_DIR" XDG_STATE_HOME="$HOME_DIR/state" "$bin" integration install pi >/dev/null
HOME="$HOME_DIR" XDG_STATE_HOME="$HOME_DIR/state" "$bin" integration install pi >/dev/null
HOME="$HOME_DIR" XDG_STATE_HOME="$HOME_DIR/state" "$bin" integration uninstall pi >/dev/null
# Skill file body should be stripped/removed; hook scripts deleted.
if [ -e "$HOME_DIR/.pi/agent/hooks/mure/on-tool-start.sh" ]; then
  fail "pi hook still present after uninstall"
fi
pass "install pi → reinstall (no-op) → uninstall leaves HOME clean"

# 4. `mure spawn` with no resolution source prints the 4-slot error.
set +e
# Isolated tmux server (no @mure-harness set) + cleared env.
isotmux="$HOME_DIR/iso-tmux.sock"
trap 'tmux -S "$isotmux" kill-server 2>/dev/null || true; rm -rf "$HOME_DIR"' EXIT
tmux -S "$isotmux" new-session -d -s iso 'sleep 30' 2>/dev/null || true
err=$(env -u MURE_HARNESS -u TMUX -u TMUX_PANE \
  MURE_TMUX_SOCKET="$isotmux" MURE_SOCKET=/tmp/no.sock \
  "$bin" spawn worker 2>&1 1>/dev/null)
rc=$?
tmux -S "$isotmux" kill-server 2>/dev/null || true
set -e
[ "$rc" -ne 0 ] || fail "spawn without harness should fail"
for slot in "--harness flag" "MURE_HARNESS env" "session @mure-harness" "global @mure-harness"; do
  echo "$err" | grep -q -- "$slot" || fail "spawn error missing slot: $slot ($err)"
done
pass "spawn 4-slot resolution error"

# 5. Removed code paths gone from the tree.
[ ! -e pi-mure ] || fail "pi-mure/ still present"
[ ! -e internal/piext ] || fail "internal/piext/ still present"
pass "pi-mure/ and internal/piext/ absent"

# 6. README mentions Adding a harness.
grep -q "Adding a harness" README.md || fail "README missing 'Adding a harness' section"
pass "README has Adding a harness"

# 7. spawn_e2e.sh covers spawn + emit + ls --json end-to-end.
if command -v tmux >/dev/null 2>&1 && command -v jq >/dev/null 2>&1; then
  bash test/spawn_e2e.sh >/dev/null
  pass "spawn_e2e.sh (env propagation + status reflected in mure ls)"
else
  printf '  skip spawn_e2e.sh (tmux or jq missing)\n'
fi

echo "acceptance: PASS"
