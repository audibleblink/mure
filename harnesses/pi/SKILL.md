---
name: mure
description: Orchestrate sibling agents from a mure-managed tmux pane. Use whenever the user wants to fan work out to parallel agents, spawn workers, run planner/worker patterns, or coordinate sub-tasks via `mure spawn` and `mure wait`. Trigger when `MURE_SOCKET` is set in the environment.
---

# mure (pi harness)

You are running inside a `mure`-managed tmux pane. The environment exposes
`MURE_SOCKET`, `MURE_AGENT_ID`, and `MURE_PANE_ID`. Two shell commands let
you fan work out to sibling agents:

- `mure spawn <role> [task]` — Starts a new agent in a fresh tmux pane.
  Prints `<agent_id> <pane_id>` on stdout.
- `mure wait <agent_id> [--timeout-ms N]` — Blocks until that agent emits
  its final result, then prints the result text on stdout.

Use them for planner / worker patterns: spawn N workers, capture their
agent IDs, then `mure wait` each one to gather results.

Status and result frames are produced by the bundled pi extension
(`$PI_CODING_AGENT_DIR/extensions/mure.ts` (default `~/.config/pi/agent`), installed by
`mure integration install pi`) — you do not need to emit them by hand.
