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

Status updates flow automatically from the pi hooks installed alongside
this skill — you do not need to emit them by hand.
