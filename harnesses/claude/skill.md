
## mure orchestration

When running inside a `mure`-managed tmux pane (`MURE_SOCKET` is set in
the environment) two shell commands are available for fanning work out:

- `mure spawn <role> [task]` — Start a sibling agent in a fresh tmux
  pane. Prints `<agent_id> <pane_id>` on stdout.
- `mure wait <agent_id>` — Block until that agent emits its final
  result, then print the result text on stdout.

To wire up status/result reporting, register the installed hook
scripts in your Claude Code settings under the `hooks` key:

```json
"hooks": {
  "PreToolUse":  [{"hooks": [{"type": "command", "command": "~/.claude/hooks/mure-pre-tool-use.sh"}]}],
  "PostToolUse": [{"hooks": [{"type": "command", "command": "~/.claude/hooks/mure-post-tool-use.sh"}]}],
  "Stop":        [{"hooks": [{"type": "command", "command": "~/.claude/hooks/mure-stop.sh"}]}]
}
```
