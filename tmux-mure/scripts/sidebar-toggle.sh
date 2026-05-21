#!/usr/bin/env bash
# Toggle the mure sidebar in the current session.
# If a pane in this session is flagged @mure-is-sidebar=1, kill it.
# Otherwise, split a new pane running `mure sidebar` and flag it.

set -eu

session=$(tmux display-message -p '#{session_id}')

# Find an existing sidebar pane in this session.
existing=$(tmux list-panes -s -t "$session" \
    -F '#{pane_id} #{@mure-is-sidebar}' 2>/dev/null \
    | awk '$2 == "1" { print $1; exit }')

if [ -n "${existing:-}" ]; then
    tmux kill-pane -t "$existing"
    exit 0
fi

if ! command -v mure >/dev/null 2>&1; then
    tmux display-message "mure-sidebar: 'mure' not found in PATH"
    exit 1
fi

width=$(tmux show-option -gqv @mure-sidebar-width)
[ -z "$width" ] && width=36
position=$(tmux show-option -gqv @mure-sidebar-position)
[ -z "$position" ] && position=left

# -f makes tmux split the full window (not just the active pane), so the
# sidebar pane spans the entire edge of the window regardless of the
# current pane layout.
case "$position" in
    right) split_args=(-h -f) ;;
    top)   split_args=(-v -b -f) ;;
    bottom) split_args=(-v -f) ;;
    left|*) split_args=(-h -b -f) ;;
esac

new_pane=$(tmux split-window -P -F '#{pane_id}' \
    -c '#{pane_current_path}' \
    "${split_args[@]}" -l "$width" "mure sidebar")

tmux set-option -p -t "$new_pane" @mure-is-sidebar 1
