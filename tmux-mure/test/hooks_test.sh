#!/usr/bin/env bash
# Integration test for tmux-mure.
# - Sources the plugin into an isolated tmux server.
# - Asserts the three hooks are registered.
# - Asserts sidebar-toggle creates a pane with @mure-is-sidebar=1,
#   then destroys it on the second invocation.
#
# Requires: tmux >= 3.2 on PATH.
set -eu

PLUGIN_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )"
SOCKET=mure-test-$$

WORK=$(mktemp -d)
TMUX_TMPDIR="$WORK"
export TMUX_TMPDIR

# Stub `mure` on PATH that just blocks; the sidebar pane must stay alive
# long enough for us to inspect it.
STUB_DIR="$WORK/bin"
mkdir -p "$STUB_DIR"
cat >"$STUB_DIR/mure" <<'EOF'
#!/usr/bin/env bash
# stub: just sleep so the pane stays open for the test.
exec sleep 30
EOF
chmod +x "$STUB_DIR/mure"
export PATH="$STUB_DIR:$PATH"

cleanup() {
    tmux -L "$SOCKET" kill-server 2>/dev/null || true
    rm -rf "$WORK"
}
trap cleanup EXIT

tmux_cmd() { tmux -L "$SOCKET" "$@"; }

fail() { echo "FAIL: $*" >&2; exit 1; }

tmux_cmd new-session -d -s test -x 200 -y 50

# Source the plugin.
tmux_cmd run-shell "$PLUGIN_DIR/tmux-mure.tmux"

# --- Assert plugin-version option set ---
ver=$(tmux_cmd show-option -gv @mure-plugin-version)
[ "$ver" = "1" ] || fail "@mure-plugin-version=$ver (want 1)"

# --- Assert hooks registered ---
for h in after-select-pane pane-exited session-closed; do
    body=$(tmux_cmd show-hooks -g "$h" 2>/dev/null || true)
    case "$body" in
        *"mure _hook"*) ;;
        *) fail "hook $h not registered (body=$body)" ;;
    esac
done

# --- Sidebar toggle: create ---
tmux_cmd run-shell "$PLUGIN_DIR/scripts/sidebar-toggle.sh"
# Give the new pane a moment to register.
sleep 0.3

sidebar_pane=$(tmux_cmd list-panes -s -t test \
    -F '#{pane_id} #{@mure-is-sidebar}' | awk '$2=="1"{print $1}')
[ -n "$sidebar_pane" ] || fail "sidebar pane not created"

# --- Sidebar toggle: destroy ---
tmux_cmd run-shell "$PLUGIN_DIR/scripts/sidebar-toggle.sh"
sleep 0.3

leftover=$(tmux_cmd list-panes -s -t test \
    -F '#{pane_id} #{@mure-is-sidebar}' | awk '$2=="1"{print $1}')
[ -z "$leftover" ] || fail "sidebar pane $leftover survived second toggle"

# --- uninstall-hooks ---
tmux_cmd run-shell "$PLUGIN_DIR/scripts/uninstall-hooks.sh"
for h in after-select-pane pane-exited session-closed; do
    body=$(tmux_cmd show-hooks -g "$h" 2>/dev/null || true)
    case "$body" in
        *"mure _hook"*) fail "hook $h still present after uninstall (body=$body)" ;;
    esac
done

echo "ok tmux-mure hooks_test"
