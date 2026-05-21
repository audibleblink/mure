# PRD 005 — Generalize Harness Support

## 1. Summary

Decouple mure from the `pi` coding-agent harness. Ship first-class
support for `pi`, `claude`, and `opencode` out of the box, and make
adding a new harness a documentation-only PR — a folder under
`harnesses/<name>/` containing a TOML manifest, a `skill.md`, and
hook scripts. No Go or TypeScript shim per harness.

## 2. Goals

- Users can choose their harness via `--harness`, `MURE_HARNESS`, or
  the tmux option `@mure-harness`.
- Adding a new harness requires **only** files under `harnesses/<name>/`;
  no changes to `cmd/`, `internal/`, or daemon code.
- Three reference harnesses ship in-tree: `pi`, `claude`, `opencode`.
- Each capability (spawn, status, result, subtools) is provided by
  declarative manifest + hook scripts + skill file — never by code in
  `internal/`.
- `pi`-specific code (`pi-mure/`, `internal/piext`) is removed; the
  `pi` harness uses the same path as every other harness.

## 3. Non-Goals

- Remote / over-the-network harnesses.
- Sandboxing or auth around `mure emit`.
- Project-scoped installs (user scope only — see §7).
- Backward-compatible bridge for the old `pi.registerTool(...)` flow
  (hard cutover — see §11).
- A GUI/TUI for managing installed harnesses beyond `mure integration
  {install,uninstall,list}`.

## 4. User Stories

1. **As a Claude Code user**, I run `mure integration install claude`,
   then `mure up` opens panes that run `claude` and report status to
   the sidebar.
2. **As a new-harness contributor**, I open a PR adding
   `harnesses/aider/{manifest.toml,skill.md,hooks/…}` and nothing
   else; reviewers verify the manifest validates and the harness
   appears in `mure integration list`.
3. **As an in-pane agent**, regardless of harness, I can call
   `mure spawn` / `mure wait` via shell, because my harness's
   instruction file (installed by `mure integration install`) tells
   me they exist.
4. **As a power user**, I set `tmux set-option @mure-harness opencode`
   on one session and `MURE_HARNESS=claude mure spawn …` on another;
   each pane launches the right binary.

## 5. Tech Stack

| Layer | Choice |
|---|---|
| Manifest format | TOML |
| Go TOML parser | `github.com/pelletier/go-toml/v2` |
| Asset bundling | `go:embed` over the entire `harnesses/` tree |
| Hook scripts | Whatever each harness's hook system expects (typically shell); mure does not constrain it |
| Daemon transport for `mure emit` | Existing unix socket / NDJSON (unchanged on the wire) |
| Pane → session linking | Env vars (`MURE_PANE_ID`, `MURE_SESSION`, `MURE_AGENT_ID`, `MURE_SOCKET`) exported into spawned panes; hooks may fall back to `tmux display -p` if needed |
| Tests | Existing Go `testing` + table tests; no new test framework |

## 6. Folder Shape

```
harnesses/<name>/
  manifest.toml      # binary, task-arg style, install plan, capabilities
  skill.md           # instructs agent that `mure spawn` / `mure wait` exist
  hooks/             # harness-native hook scripts; each calls `mure emit ...`
```

The whole `harnesses/` directory is embedded into the mure binary via
`go:embed`. `internal/harnesses` is the only Go package that knows
about the registry.

## 7. Manifest Schema (TOML)

```toml
# harnesses/<name>/manifest.toml
name        = "claude"
display     = "Claude Code"
command     = "claude"            # binary exec'd in the new pane
task_arg    = "flag:--prompt"     # positional | stdin | flag:<name> | none

[capabilities]
spawn   = true
status  = true                    # via hooks
result  = true                    # via hooks
subtools = true                   # via skill.md

[install.skill]
path  = "~/.claude/CLAUDE.md"
merge = "append"                  # append | replace | create-if-missing

[[install.hooks]]
src   = "hooks/pre-tool.sh"
dst   = "~/.claude/hooks/pre-tool.sh"
mode  = 0o755
```

Notes:

- Paths are user-scope only (typically under `$HOME`); `~` is expanded.
- `task_arg` values:
  - `positional` — task is appended as a single argv element
  - `stdin` — task is written to the child's stdin and stdin is closed
  - `flag:<name>` — task is passed as `<name>=<task>` (single argv el)
  - `none` — task is dropped; harness picks up work via other means
- `capabilities` are declarative; missing/false capabilities surface in
  `mure integration list` and in CLI error messages (see §10).
- Unknown TOML keys are an install-time error (strict decode).

## 8. CLI Surface

### `mure integration install <name>`
- Resolves manifest from the embedded registry.
- Computes a file plan from `install.skill` + `install.hooks`.
- Applies the plan with the declared merge modes.
- Idempotent: re-running install reconciles to the manifest state.

### `mure integration uninstall <name>`
- Reverses files mure wrote (tracked via a small state file under
  `$XDG_STATE_HOME/mure/integrations/<name>.json` recording
  destinations + content hashes; merge=`append` blocks are demarcated
  with begin/end markers and removed cleanly).

### `mure integration list`
- Prints embedded harnesses, install state, and capability matrix.

### `mure emit <kind> [args]`
- New canonical NDJSON producer. Subkinds at minimum:
  - `mure emit status <working|blocked|idle> [--tool <name>]`
  - `mure emit result -`  (reads result text from stdin)
- Reads `MURE_SOCKET` / `MURE_PANE_ID` / `MURE_SESSION` from env;
  falls back to tmux lookup if env vars are missing.
- The NDJSON wire format becomes an internal detail of mure; hook
  authors only need `mure emit …`.

### `mure spawn` (updated)
Harness resolution order:
1. `--harness <name>` flag
2. `MURE_HARNESS` env var
3. tmux option `@mure-harness` on the current session
4. **error** — no implicit default (per user decision)

Once resolved, `mure spawn` reads the manifest's `command` and
`task_arg` and launches accordingly.

### tmux integration
- `@mure-harness` is the session-level default.
- Flag and env override per-spawn.

## 9. Subtools via Skill Files

`mure_spawn` and `mure_wait` are *not* native tools. They are the CLI
commands `mure spawn` and `mure wait`. Each harness's `skill.md`,
installed at the harness's documented instruction-file path (e.g.
`~/.claude/CLAUDE.md`, the pi skills dir, opencode's `AGENTS.md`),
teaches the agent to shell out to them.

No native tool registration, no per-harness TS/Go shim.

## 10. Status & Result via Hooks + `mure emit`

Each harness ships hook scripts under `harnesses/<name>/hooks/` (or a
harness-native plugin file) that the harness's native lifecycle hooks
invoke on tool-call start/end, turn end, etc. Those scripts exec
`mure emit …` to push status/result frames to the daemon.

### Graceful Degradation — DEFERRED

The original design called for a `tmux capture-pane` fallback when a
harness declared `capabilities.{status,result}=false`. **This is not
implemented and is deferred to a follow-up.** All three reference
harnesses (`pi`, `claude`, `opencode`) ship with hook-based status &
result, so the fallback has no current consumer.

In the interim:
- A harness with `capabilities.status=false` or `capabilities.result=false`
  will produce **no** status / result updates; agents stay at the status
  last emitted (typically `idle` from spawn).
- `mure integration list` labels such harnesses `degraded` so the
  condition is visible.
- Future implementation will follow §10's original sketch (poll loop
  + `degraded: true` marker on results).

Manifest `capabilities.{status,result}=false` is reserved for the
fallback; do not set them false on a harness intended to ship in this
release.

## 11. Migration (Hard Cutover)

- Move `pi-mure/` into `harnesses/pi/` in the new shape.
- Delete `internal/piext` (replaced by `internal/harnesses` with
  `go:embed` over the tree).
- Remove all `pi.registerTool(...)` code; the `pi` harness uses the
  same skill-file + hooks path as Claude/opencode.
- Update `cmd/mure/spawn.go` and `cmd/mure/integration.go` to consult
  the harness registry.
- No deprecation window; this PR is the cutover.

## 12. Files Touched / Added

- **New:** `harnesses/{pi,claude,opencode}/{manifest.toml,skill.md,hooks/…}`
- **New:** `internal/harnesses/` — embed FS, manifest loader, registry,
  install planner, capability resolution.
- **New:** `cmd/mure/emit.go` — `mure emit` subcommand.
- **Modified:** `cmd/mure/spawn.go` — harness resolution.
- **Modified:** `cmd/mure/integration.go` — drive from registry.
- **Modified:** daemon — accept `mure emit` frames (no wire change;
  rename internal symbols if needed).
- **Modified:** `README.md` — "Adding a harness" section.
- **Deleted:** `pi-mure/`, `internal/piext/`.

## 13. Out-of-Scope Follow-Ups

- Project-scoped (`./`) integration installs.
- `mure emit` auth / per-pane capability tokens.
- Streaming intermediate frames beyond status/result.
- Harness auto-detect from `$PATH`.
- Web/remote harness adapters.

## 14. Acceptance Criteria

1. `harnesses/{pi,claude,opencode}/manifest.toml` exist and validate.
2. `mure integration list` shows the three harnesses and their
   install state + capabilities.
3. `mure integration install pi` writes the expected files; running
   it twice is a no-op; `uninstall` removes them.
4. `mure spawn` with no resolution source errors clearly listing the
   four resolution slots.
5. `mure spawn --harness claude <role>` launches `claude` in a new
   pane and propagates `MURE_PANE_ID`/`MURE_SESSION` into its env.
6. A hook calling `mure emit status working --tool foo` updates the
   sidebar status for the correct pane.
7. A hook calling `mure emit result -` makes `mure wait` return that
   text.
8. ~~For a harness with `capabilities.status=false`, status reflects the
   `tmux capture-pane` fallback and `mure integration list` labels
   it degraded.~~ **Deferred** (see §10). `mure integration list` still
   labels such harnesses `degraded`, but no fallback runs.
9. `pi-mure/` and `internal/piext/` are removed from the tree; build
   succeeds.
10. README documents how to add a harness in <1 page.

## 15. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Hooks vary wildly between harnesses | Manifest only declares *where* files go; hook content is per-harness and reviewed in the PR |
| `tmux capture-pane` fallback is noisy / unreliable | Mark degraded results with `degraded: true`; document heuristic; do not use fallback when hooks declared |
| `mure emit` called outside a mure pane | `emit` requires `MURE_SOCKET`; errors loudly otherwise |
| Manifest schema drift over time | Strict TOML decode + a `manifest_version` field reserved for future bumps |
| Hard cutover breaks existing pi users | Documented in README + release notes; pi users re-run `mure integration install pi` post-upgrade |
