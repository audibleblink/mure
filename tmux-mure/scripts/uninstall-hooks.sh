#!/usr/bin/env bash
# Remove the global tmux hooks installed by tmux-mure.
set -eu

tmux set-hook -gu after-select-pane
tmux set-hook -gu pane-exited
tmux set-hook -gu session-closed
