---
name: mure
description: Orchestrate sibling agents from a mure-managed tmux pane. Use whenever the user wants to fan work out to parallel agents, spawn workers, run planner/worker patterns, or coordinate sub-tasks via `mure spawn` and `mure wait`. Trigger when `MURE_SOCKET` is set in the environment.
hooks:
  PreToolUse:
    - hooks:
        - type: command
          command: "${CLAUDE_PLUGIN_ROOT:-$HOME/.claude/skills/mure}/scripts/pre-tool-use.sh"
  PostToolUse:
    - hooks:
        - type: command
          command: "${CLAUDE_PLUGIN_ROOT:-$HOME/.claude/skills/mure}/scripts/post-tool-use.sh"
  Stop:
    - hooks:
        - type: command
          command: "${CLAUDE_PLUGIN_ROOT:-$HOME/.claude/skills/mure}/scripts/stop.sh"
---

## mure orchestration

When running inside a `mure`-managed tmux pane (`MURE_SOCKET` is set in
the environment) two shell commands are available for fanning work out:

- `mure spawn <role> [task]` — Start a sibling agent in a fresh tmux
  pane. Prints `<agent_id> <pane_id>` on stdout.
- `mure wait <agent_id>` — Block until that agent emits its final
  result, then print the result text on stdout.

Status and result frames are produced by the hook scripts declared in
this skill's frontmatter — you do not need to emit them by hand.
