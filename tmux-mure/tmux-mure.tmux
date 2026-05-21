#!/usr/bin/env bash
# tmux-mure: tmux-side surfaces for mure (hooks, decoration, sidebar).
# Owns: hooks, status-line snippet, pane decoration, sidebar toggle, spawn-target.
# Spawns no processes of its own.

set -eu

CURRENT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

tmux_get() {
    # tmux_get <option> <default>
    local val
    val=$(tmux show-option -gqv "$1" 2>/dev/null || true)
    if [ -z "$val" ]; then
        printf '%s' "$2"
    else
        printf '%s' "$val"
    fi
}

# Plugin version marker (read by `mure doctor`).
tmux set-option -g @mure-plugin-version 1

# ---- Hooks (PRD §11.1) ----
# Unset first so re-sourcing the plugin (config reload) does not duplicate or
# silently overwrite user-defined hooks layered on top.
tmux set-hook -gu after-select-pane
tmux set-hook -gu pane-exited
tmux set-hook -gu session-closed

# Resolve `mure` once at plugin load; bake the absolute path into hook
# commands so the hottest tmux events (every pane focus / exit) don't fork a
# shell just to run `command -v`. If mure isn't on PATH at load time, skip
# hook installation entirely.
MURE_BIN="$(command -v mure || true)"
if [ -n "$MURE_BIN" ]; then
  tmux set-hook -g after-select-pane \
    "run-shell -b '$MURE_BIN _hook focus #{pane_id} #{client_name} || true'"

  tmux set-hook -g pane-exited \
    "run-shell -b '$MURE_BIN _hook pane_died #{pane_id} || true'"

  tmux set-hook -g session-closed \
    "run-shell -b '$MURE_BIN _hook session_closed #{hook_session} || true'"
fi

# Agent status is intentionally NOT surfaced via pane-border-format or a
# status-line snippet. Status is observable only via `mure ls` and the
# sidebar (which reads from the daemon socket).

# ---- Sidebar + spawn defaults ----
tmux set-option -g @mure-sidebar-width    "$(tmux_get @mure-sidebar-width    36)"
tmux set-option -g @mure-sidebar-position "$(tmux_get @mure-sidebar-position left)"
tmux set-option -g @mure-sidebar-key      "$(tmux_get @mure-sidebar-key      M)"
# @mure-spawn-target is either the reserved keyword `subagents-window`
# (find-or-create a dedicated window in this session) or an arbitrary tmux
# command that creates a pane — mure appends `-P -F #{pane_id} <payload>`.
# Legacy keyword values are rewritten here so users keep their existing
# config, and so all behavior definitions live in the plugin where users
# can override them.
spawn_target_raw="$(tmux_get @mure-spawn-target subagents-window)"
case "$spawn_target_raw" in
    right-of-active) spawn_target="split-window -h" ;;
    below-active)    spawn_target="split-window -v" ;;
    new-window)      spawn_target="new-window" ;;
    *)               spawn_target="$spawn_target_raw" ;;
esac
tmux set-option -g @mure-spawn-target "$spawn_target"

# ---- Sidebar toggle bind ----
sidebar_key="$(tmux_get @mure-sidebar-key M)"
tmux bind-key -T prefix "$sidebar_key" run-shell "$CURRENT_DIR/scripts/sidebar-toggle.sh"
