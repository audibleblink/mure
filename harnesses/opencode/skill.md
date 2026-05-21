
## mure orchestration

When running inside a `mure`-managed tmux pane (`MURE_SOCKET` is set in
the environment) two shell commands are available for fanning work out:

- `mure spawn <role> [task]` — Start a sibling agent in a fresh tmux pane.
  Prints `<agent_id> <pane_id>` on stdout.
- `mure wait <agent_id>` — Block until that agent emits its final result,
  then print the result text on stdout.

Note: until a dedicated opencode plugin is published, mure infers this
pane's activity from its tmux output (capture-pane fallback) rather than
from opencode lifecycle events — the sidebar may show `(degraded)`.
