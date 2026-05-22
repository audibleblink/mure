# mure — Current State

Go ≥ 1.24 (toolchain 1.24.5). tmux ≥ 3.2.
Module: `github.com/audibleblink/mure`. No tagged release yet.

Direct Go deps: `charmbracelet/bubbletea v1.3.10`, `charmbracelet/lipgloss v1.1.0`,
`muesli/termenv v0.16.0`, `golang.org/x/sys v0.36.0`,
`pelletier/go-toml/v2 v2.3.1`.

## What mure Is

A tmux-native multiplexer for **coding-agent panes**. The platform layer is a
Unix-socket daemon plus a thin CLI; everything else (a Bubble Tea sidebar,
harness integrations) plugs into the same socket protocol or reads the same
`@mure-*` tmux options. One binary, two contracts:

1. **NDJSON wire protocol** (`internal/sock`) over a per-session Unix socket.
2. **`@mure-*` tmux options** read (and a few pane-scoped ones written) by
   `cmd/mure`.

The daemon never edits tmux config beyond pane-scoped marker options. Use
case layer (agent orchestration via skill files, sidebar visuals) is built
on top.

## Architecture

| Piece | Path | Language | Role |
|---|---|---|---|
| `mure` CLI + daemon | `cmd/mure`, `internal/daemon` | Go | Owns the socket, roster, tmux bridge. |
| Sidebar TUI | `internal/sidebar` | Go (Bubble Tea) | `mure sidebar` pane — subscribes to roster diffs. |
| Wire protocol | `internal/sock` | Go | NDJSON frame types + framer; `MaxFrameSize=64 KiB`, `ProtocolVersion=1`. |
| tmux control client | `internal/tmuxctl` | Go | Wraps `tmux -C` for the daemon bridge. |
| Harness system | `internal/harnesses`, `harnesses/` | Go + TOML | Manifest-driven install/uninstall for agent integrations. Embedded via `go:embed`. |


### CLI verbs (`cmd/mure/main.go`)

`up`, `down`, `ls [--json]`, `spawn <role> [task]`, `wait <agent>`,
`focus <agent>`, `sidebar`, `emit`, `doctor`,
`integration {list,install,uninstall} <name>`.

### Daemon subsystems (`internal/daemon/daemon.go`)

`Logger` → `Roster` → `tmuxbridge` (control-mode client; pane-died →
roster.Remove) → Unix-socket `server`. Peer-authentication is OS-specific
(`peerauth_{darwin,linux,other}.go`).

### Spawn targets (`cmd/mure/spawn_target.go`)

`@mure-spawn-target` accepts the reserved keyword `subagents-window` (default,
empty string is equivalent) which triggers find-or-create of a dedicated
`subagents` window — new panes go in via `split-window -h` and a
`select-layout even-horizontal` PostCreate keeps the columns balanced. Any
other value is treated as a tmux command template: mure splits on whitespace
and appends `-P -F '#{pane_id}' <payload>` (e.g. `split-window -h`,
`new-window`, `split-window -h -f -l 40%`).

## Wire Protocol (frame types in `internal/sock/types.go`)

| Frame | Direction | Notes |
|---|---|---|
| `Hello` | agent/sidebar/cli → daemon | First frame on every connection; `role` field. `Oneshot: true` skips lifecycle tracking (used by `mure emit`). |
| `Status` | agent → daemon | `idle`/`working`/`blocked`. |
| `Bye` | agent → daemon | Clean shutdown. |
| `Result` | agent → daemon | Final text at agent turn end. |
| `Wait` | cli → daemon | Block until agent has result. |
| `Roster` | daemon → sidebar | Full snapshot. |
| `AgentUpdate` | daemon → sidebar | Single-agent diff (or deletion). |
| `Envelope` | cli → daemon | Generic control (shutdown, snapshot). |

## Harnesses (`harnesses/`, `internal/harnesses/`)

Each harness is a directory under `harnesses/<name>/` containing a
`manifest.toml`, optional hook scripts / TS plugins, and a `SKILL.md`.
The entire tree is embedded into the binary via `go:embed`.

Three harnesses ship in-tree:

| Harness | Hook mechanism | Skill delivery |
|---|---|---|
| `claude` | Shell-script Claude Code plugin hooks (`UserPromptSubmit`, `PostToolUse`, `PermissionRequest`, `Stop`) | `[[install.files]]` into `~/.claude/plugins/mure/` |
| `opencode` | TypeScript opencode plugin (`tool.execute.before/after`, `session.idle`, `permission.*`) | `[install.skill]` |
| `pi` | TypeScript pi extension (`before_agent_start`, `tool_execution_end`, `agent_end`) | `[install.skill]` |

**Manifest fields:** `manifest_version`, `name`, `display`, `command`,
`task_arg` (`positional|stdin|flag:<name>|none`), `[capabilities]`
(`spawn`, `status`, `result`, `subtools`), `[install.skill]` (optional),
`[[install.files]]` list.

**Merge strategies:** `replace`, `create-if-missing`, `append` (idempotent
marker-block wrapping `# >>> mure:<name> >>>`).

**`mure emit`** — thin CLI used by shell-script hooks to write a single
oneshot NDJSON frame to the daemon without holding a lifecycle connection.

## Agent Orchestration (Skills)

Harnesses with `subtools = true` install a `SKILL.md` that teaches the
agent to use two shell commands:

- `mure spawn <role> [task]` — fan out a sibling agent in a new pane.
- `mure wait <agent_id>` — block until that agent emits its final result.

The skill is only present inside mure-managed panes; agents running outside
mure never see it.

## Configuration

### Environment variables

| Var | Default | Purpose |
|---|---|---|
| `MURE_SESSION` | tmux `#S` else `default` | Namespaces runtime dir + socket. |
| `MURE_SOCKET` | `<runDir>/daemon.sock` | Override socket path. |
| `MURE_RUN_DIR` | per-OS cache/runtime dir | Override per-session runtime dir. |
| `MURE_TMUX_SOCKET` | parsed from `$TMUX` | tmux server socket. |
| `MURE_AGENT_ID` | — | Set by `mure spawn` inside agent pane. |
| `MURE_AGENT_CMD` | — | Command `mure spawn` exec's as the agent. |
| `MURE_TASK` | — | Initial task label. |
| `MURE_ENV` | — | Comma-separated extra env into spawned panes. |
| `MURE_DAEMON` | unset | Internal — set on the forked daemon process. |

Runtime dir: `~/Library/Caches/mure/<session>/` on macOS,
`$XDG_RUNTIME_DIR/mure/<session>/` (fallback `/tmp/mure-<uid>/<session>/`) on
Linux. Forced `0700`.

### tmux options (`@mure-*`)

Read by `cmd/mure` from the user's `~/.tmux.conf`:
`@mure-sidebar-width` (default `36`), `@mure-sidebar-position` (default
`left`), `@mure-spawn-target` (default `subagents-window`),
`@mure-harness` (per-session or global, optional).

## Build & Test

`make build` (`go build`), `make test` (`go test ./...`), `make lint`
(`go vet` + gofmt), `make acceptance` (real tmux integration test),
`make verify` (all).

### Test counts

| Package | Tests | Status |
|---|---:|---|
| `cmd/mure` | 25 | ok |
| `internal/daemon` | 19 | ok |
| `internal/harnesses` | 21 | ok |
| `internal/sidebar` | 27 | ok |
| `internal/sock` | 6 | ok |
| `internal/tmuxctl` | 7 | ok |
| `test/protocol` | 1 | ok |
| **Go total** | **106** | all passing |

Additional standalone Go tests live in `test/e2e/e2e_test.go` and
`test/throughput/throughput_test.go`; the e2e suite has a `stubagent` helper
under `test/e2e/stubagent/` and is run as part of `go test ./...`.

## Known Issues / Tech Debt

- No first-party `TODO`/`FIXME`/`HACK` comments in Go or shell sources.
- No tagged release yet; README mentions `brew install` once a release is tagged.
- License is `TBD` in the top-level README.


## Linear Progression

### 001 — init (`specs/001-init/`)

**Goal.** Define the three-deliverable monorepo (daemon+CLI, pi extension,
tmux plugin) and the two contracts (socket protocol, `@mure-*` options) that
let them evolve independently.
**Choices.** NDJSON over per-session Unix socket; daemon owns roster, plugin
owns tmux surfaces; neither side mutates the other.
**Result.** Established `cmd/mure`, `internal/daemon`, `internal/sock`,
`internal/sidebar`, `internal/tmuxctl`, `pi-mure/`, and the verb set +
frame catalog still in use today. The original `tmux-mure/` plugin
directory has since been removed; its sidebar toggle moved into
`mure sidebar --toggle`.

### 002 — sidebar pizzazz (`specs/002-sidebar-pizzazz/`)

**Goal.** Visual/behavior refresh of `mure sidebar` only — no protocol
changes.
**Choices.** Catppuccin-derived adaptive palette, gradient header, footer
hints, dividers, gated spinner; palette + logo kept as package-level vars in
`brand.go` / `theme.go` so later tmux-option theming is possible.
**Result.** `internal/sidebar/{brand,theme,ansi}.go` plus tests; no daemon,
sock, or plugin change.

### 003 — pi-mure tools (`specs/003-pi-mure-tools/`)

**Goal.** Let in-pane pi agents orchestrate siblings without re-implementing
the socket protocol.
**Choices.** Thin tool wrappers around `mure spawn` / `mure wait`; register
only when `MURE_ENV` and `MURE_AGENT_ID` indicate the pi process lives in a
mure pane; no new sock frames or CLI verbs.
**Result.** `mure_spawn` and `mure_wait` tools in `pi-mure/index.ts` with
gate / lifecycle / cross-tool tests under `pi-mure/test/`.

### 004 — `subagents-window` spawn target (`specs/004-subagents-window/`)

**Goal.** Collect spawned subagent panes into one tmux window per session
instead of fragmenting the current window.
**Choices.** New `@mure-spawn-target=subagents-window` value (now the
default); inline target switch extracted to `pickSpawnTarget` behind a
`tmuxRunner` seam for unit-testing; zero daemon / sock / roster / sidebar /
plugin-runtime changes (option-value docs only).
**Result.** `cmd/mure/spawn_target.go` + `spawn_target_test.go`; lazy
`subagents` window creation in `cmd/mure/spawn.go`.

### 005 — generalize harness support (`specs/005-generalize-harness/`)

**Goal.** Decouple mure from the `pi` harness; ship first-class support for
`pi`, `claude`, and `opencode`; make adding a new harness a documentation-only
PR (a folder under `harnesses/<name>/`).
**Choices.** TOML manifest per harness; `go:embed` over the entire
`harnesses/` tree; `mure emit` CLI for shell-hook–based harnesses; skill files
replace `pi.registerTool(...)` for orchestration; `pi-mure/` and
`internal/piext` removed.
**Result.** `internal/harnesses/`, `harnesses/{claude,opencode,pi}/`,
`cmd/mure/integration.go`, `cmd/mure/emit.go`; three reference harnesses
each emitting status/result frames and installing a skill for `mure spawn` /
`mure wait`.

### The Arc

001 set the rule (two contracts, three pieces, nobody crosses lines).
002 polished the only user-facing TUI surface within its lane.
003 turned the daemon into a fan-out primitive by giving pi agents tools.
004 made the multi-agent UX livable by quarantining subagent panes.
005 generalized the harness layer — any agent runtime can integrate without
touching Go, and orchestration works via installed skills rather than
registered tools.

Each spec stays in exactly one layer. That is the project's discipline.
