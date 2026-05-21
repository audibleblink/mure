#!/usr/bin/env bash
# End-to-end check for `mure integration {install,list,uninstall}`.
# Uses MURE_HARNESSES_DIR to inject a synthetic harness without touching
# the embedded tree.
set -euo pipefail

repo="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo"

go build -o bin/mure ./cmd/mure

HOME_DIR="$(mktemp -d)"
HARNESS_DIR="$(mktemp -d)"
trap 'rm -rf "$HOME_DIR" "$HARNESS_DIR"' EXIT

mkdir -p "$HARNESS_DIR/_test"
cat >"$HARNESS_DIR/_test/manifest.toml" <<'TOML'
name = "_test"
display = "Test"
command = "echo"
[capabilities]
spawn = true
status = true
result = true
[install.skill]
path = "~/skill.md"
merge = "append"
[[install.files]]
src = "h.sh"
dst = "~/h.sh"
mode = "0755"
TOML
echo "skill-body" >"$HARNESS_DIR/_test/SKILL.md"
echo "#!/bin/sh" >"$HARNESS_DIR/_test/h.sh"

unset PI_CODING_AGENT_DIR XDG_CONFIG_HOME MURE_HARNESS
export HOME="$HOME_DIR"
export XDG_STATE_HOME="$HOME_DIR/state"
export MURE_HARNESSES_DIR="$HARNESS_DIR"

./bin/mure integration install _test
./bin/mure integration list | grep -q '_test'
# Idempotent re-install.
./bin/mure integration install _test
./bin/mure integration uninstall _test

# Everything should be gone.
if [[ -e "$HOME_DIR/h.sh" ]]; then
  echo "hook file still present" >&2
  exit 1
fi
if [[ -e "$HOME_DIR/skill.md" ]]; then
  echo "skill file not stripped" >&2
  exit 1
fi
if [[ -e "$XDG_STATE_HOME/mure/integrations/_test.json" ]]; then
  echo "state not cleared" >&2
  exit 1
fi

echo "harness_install.sh: ok"
