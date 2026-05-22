# Harnesses

A *harness* is a coding-agent CLI that mure knows how to launch and listen to.
Each one lives under `harnesses/<name>/` and is embedded into the mure binary
at build time via `go:embed`.

## In-tree harnesses

| Name | Agent | Status/result delivery |
|---|---|---|
| `claude` | Claude Code | Shell-script plugin hooks (`UserPromptSubmit`, `PostToolUse`, `PermissionRequest`, `Stop`) calling `mure emit` |
| `opencode` | opencode | TypeScript plugin (`tool.execute.before/after`, `session.idle`, `permission.*`) |
| `pi` | pi | TypeScript extension (`before_agent_start`, `tool_execution_end`, `agent_end`) |

## Adding a harness

1. **Create the folder.** `harnesses/<name>/` — `<name>` is what users type in
   `mure integration install <name>` and `mure spawn --harness <name>`.

2. **Write `manifest.toml`.** See the [full schema](#manifest-schema) below.
   Required fields: `name`, `command`, `task_arg`. Declare `[capabilities]`
   honestly — `status` and `result` should be `true` only if the harness emits
   frames via a hook or plugin. Declaring them `false` means the harness emits
   no status or result frames; `mure integration list` will label it `degraded`.

3. **Drop the bridge.** Hook scripts, TS plugins, or any file the harness needs
   on disk go under `[[install.files]]`. Each entry has `src` (path inside the
   harness folder), `dst` (install target, `~` expanded), and `mode`.
   Shell hooks use `0755`; plugins and data files use `0644`.

   Shell hooks must guard against `mure` being absent (the harness may be
   invoked outside a mure session). Open every hook script with:
   ```sh
   command -v mure >/dev/null 2>&1 || exit 0
   ```

   Each hook ultimately calls one of:
   ```sh
   mure emit status working [--tool <name>]
   mure emit status blocked
   mure emit status idle
   mure emit result -          # reads result text from stdin
   ```

4. **Optional skill file.** `SKILL.md` teaches the agent that `mure spawn` /
   `mure wait` exist. Declare it either as a `[install.skill]` block or as a
   plain `[[install.files]]` entry — both work. Set `subtools = true` in
   `[capabilities]` when the skill is present.

5. **Test locally.**
   ```sh
   make build
   ./bin/mure integration install <name>
   mure integration list
   ./bin/mure integration uninstall <name>
   ```

6. **Open a PR.** CI validates every manifest under `harnesses/` via the
   `internal/harnesses` test suite (strict TOML decode, required-field checks).

## Manifest schema

```toml
manifest_version = 1            # always 1 for now
name    = "myharness"           # matches the directory name
display = "My Harness"          # shown in `mure integration list`
command = "myharness"           # binary exec'd in the new pane

# How the task string is passed to the agent process:
#   positional  — appended as a bare argv element
#   stdin       — written to stdin, then stdin is closed
#   flag:<name> — passed as <name> <task> (two argv elements)
#   none        — task is dropped
task_arg = "positional"

[capabilities]
spawn    = true   # agent can call `mure spawn` (skill teaches the shell commands)
status   = true   # harness emits status frames (working/blocked/idle)
result   = true   # harness emits a result frame on turn end
subtools = true   # skill file is installed; set false if no SKILL.md

# [install.skill] — optional; alternative to listing the skill under [[install.files]]
[install.skill]
path  = "~/.config/myharness/skills/mure/SKILL.md"
merge = "replace"   # append | replace | create-if-missing

# [[install.files]] — one block per file to install
[[install.files]]
src  = "hooks/on-tool-end.sh"
dst  = "~/.config/myharness/hooks/on-tool-end.sh"
mode = "0755"

[[install.files]]
src  = "hooks/on-turn-end.sh"
dst  = "~/.config/myharness/hooks/on-turn-end.sh"
mode = "0755"
```

### `task_arg` values

| Value | Behaviour |
|---|---|
| `positional` | Task appended as a single bare argument: `myharness <task>` |
| `stdin` | Task written to the process's stdin; stdin closed immediately after |
| `flag:<name>` | Task passed as two elements: `myharness <name> <task>` |
| `none` | Task string is ignored; harness picks up work another way |

### Merge strategies for `[install.skill]` and `[[install.files]]`

| Strategy | Behaviour |
|---|---|
| `replace` | Overwrite the destination file. Uninstall removes it only if the content hash still matches. |
| `create-if-missing` | Write the file only if it does not already exist. Uninstall is a no-op. |
| `append` | Append a clearly marked block (`# >>> mure:<name> >>>` … `# <<< mure:<name> <<<`) to an existing file. Uninstall strips the block. Re-running install is idempotent. |

### Unknown keys

The manifest decoder is strict — unknown TOML keys are an error. This keeps
manifests honest and makes future schema additions detectable.
