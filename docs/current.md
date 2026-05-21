# mure — Current State

Go ≥ 1.24 (toolchain 1.24.5). tmux ≥ 3.2 (README says 3.3 for build prereq;
plugin README states 3.2 for the hook/format features it uses).
No tagged release yet; `pi-mure` package version `0.0.0`.

Direct Go deps: `charmbracelet/bubbletea v1.3.10`, `charmbracelet/lipgloss v1.1.0`,
`golang.org/x/sys v0.36.0`. Node deps (dev only): `tsx`, `typebox`, `typescript`,
`@types/node`; peer/runtime API from `@earendil-works/pi-coding-agent`.

## What mure Is

A tmux-native multiplexer for **coding-agent panes**. The platform layer is a
Unix-socket daemon plus a thin CLI; everything else (a Bubble Tea sidebar, a pi
extension, a tmux plugin) plugs into the same socket protocol or reads the same
`@mure-*` pane options. Three deliverables in one monorepo meeting at two
contracts:

1. **NDJSON wire protocol** (`internal/sock`) over a per-session Unix socket.
2. **`@mure-*` pane options** written by the daemon, read by tmux.

The daemon never edits tmux config; the plugin never spawns long-lived
processes. Use case layer (agent orchestration via `pi-mure` tools, sidebar
visuals) is built on top.

## Architecture

| Piece | Path | Language | Role |
|---|---|---|---|
| `mure` CLI + daemon | `cmd/mure`, `internal/daemon` | Go | Owns the socket, roster, tmux bridge. |
| Sidebar TUI | `internal/sidebar` | Go (Bubble Tea) | `mure sidebar` pane — subscribes to roster diffs. |
| Wire protocol | `internal/sock` | Go | NDJSON frame types + framer; `MaxFrameSize=64 KiB`, `ProtocolVersion=1`. |
| tmux control client | `internal/tmuxctl` | Go | Wraps `tmux -C` for the daemon bridge. |
| Embedded pi extension | `internal/piext` | Go | `embed.FS` mirror of `pi-mure/` (synced by `make sync-piext`). |
| pi extension | `pi-mure/` | TypeScript | Emits hello/status/result/bye; registers `mure_spawn` / `mure_wait` tools. |
| tmux plugin | `tmux-mure/` | shell + tmux | Hooks, sidebar toggle, spawn-target. |

### CLI verbs (`cmd/mure/main.go`)

`up`, `down`, `ls [--json]`, `spawn <role> [task]`, `wait <agent>`,
`focus <agent>`, `sidebar`, `doctor`, `integration {install,uninstall} pi`,
and internal `_hook`.

### Daemon subsystems (`internal/daemon/daemon.go`)

`Logger` → `Roster` → `Coalescer` (status-update window) → `Debouncer`
(disconnect/remove window) → `tmuxbridge` (control-mode reader + writer) →
Unix-socket `server`. Peer-authentication is OS-specific
(`peerauth_{darwin,linux,other}.go`).

### Spawn targets (`cmd/mure/spawn_target.go`)

`@mure-spawn-target` accepts the reserved keyword `subagents-window` (default,
empty string is equivalent) which triggers find-or-create of a dedicated
`subagents` window — new panes go in via `split-window -h` and a
`select-layout even-horizontal` PostCreate keeps the columns balanced. Any
other value is treated as a tmux command template: mure splits on whitespace
and appends `-P -F '#{pane_id}' <payload>` (e.g. `split-window -h`,
`new-window`, `split-window -h -f -l 40%`). The legacy keywords
`right-of-active` / `below-active` / `new-window` are rewritten to command
templates by the tmux plugin at load time and do not appear in the Go code.

## Wire Protocol (frame types in `internal/sock/types.go`)

| Frame | Direction | Notes |
|---|---|---|
| `Hello` | agent/sidebar/cli/hook → daemon | First frame on every connection; `role` field. |
| `Status` | agent → daemon | `idle`/`working`/`blocked`/`disconnected`/`errored`. |
| `Bye` | agent → daemon | Clean shutdown. |
| `Result` | agent → daemon | Final text at `agent_end`. |
| `Wait` | cli → daemon | Block until agent has result or errors. |
| `Focus` | hook → daemon, daemon → agent | Pane focus state. |
| `PaneDied` | hook → daemon | tmux pane exited. |
| `SessionClosed` | hook → daemon | tmux session ended. |
| `Roster` | daemon → sidebar | Full snapshot. |
| `AgentUpdate` | daemon → sidebar | Single-agent diff (or deletion). |
| `Envelope` | cli → daemon | Generic control (shutdown, snapshot). |

## pi-mure Tools

Registered only when `MURE_ENV=1` and `MURE_AGENT_ID` are set:

| Tool | Wraps | Purpose |
|---|---|---|
| `mure_spawn` | `mure spawn` | Fan out a sibling agent in a new pane. |
| `mure_wait` | `mure wait` | Block on a sibling's final result. |

## Configuration

### Environment variables (from README + grep of `os.Getenv`)

| Var | Default | Purpose |
|---|---|---|
| `MURE_SESSION` | tmux `#S` else `default` | Namespaces runtime dir + socket. |
| `MURE_SOCKET` | `<runDir>/daemon.sock` | Override socket path. |
| `MURE_RUN_DIR` | per-OS cache/runtime dir | Override per-session runtime dir. |
| `MURE_TMUX_SOCKET` | parsed from `$TMUX` | tmux server socket. |
| `MURE_AGENT_ID` | — | Set by `mure spawn` inside agent pane. |
| `MURE_AGENT_CMD` | — | Command `mure spawn` exec's as the agent. |
| `MURE_TASK` | — | Initial task label. |
| `MURE_ENV` | — | Comma-separated extra env into spawned panes; also gates pi-mure tool registration. |
| `MURE_DAEMON` | unset | Internal — set on the forked daemon. |

Runtime dir: `~/Library/Caches/mure/<session>/` on macOS,
`$XDG_RUNTIME_DIR/mure/<session>/` (fallback `/tmp/mure-<uid>/<session>/`) on
Linux. Forced `0700`.

### tmux plugin options (`@mure-*`)

`@mure-sidebar-width=36`, `@mure-sidebar-position=left`,
`@mure-sidebar-key=M`, `@mure-spawn-target=subagents-window`, and the
plugin-written `@mure-plugin-version=1`. The plugin does **not** set
`pane-border-format` or any per-status colors — agent state is observable
via `mure ls` and the sidebar only.

## Build & Test

`make build` (sync-piext + `go build`), `make test` (`go test ./...` +
shellcheck), `make lint` (`go vet` + gofmt), `make tmux-test` (real tmux hook
test, skipped if tmux missing), `make verify` (all).

### Test counts

| Package | Tests | Status |
|---|---:|---|
| `cmd/mure` | 22 | ok |
| `internal/daemon` | 24 | ok |
| `internal/sidebar` | 26 | ok |
| `internal/sock` | 6 | ok |
| `internal/tmuxctl` | 6 | ok |
| `test/protocol` | 1 | ok |
| **Go total** | **85** | all passing |
| `pi-mure` (node --test) | 23 | all passing |

Additional standalone Go tests live in `test/e2e/e2e_test.go` and
`test/throughput/throughput_test.go`; the e2e suite has a `stubagent` helper
under `test/e2e/stubagent/` and is run as part of `go test ./...` when its
build tags / environment allow.

## Skills / Knowledge / Models

N/A — this is a Go/TS infrastructure project, not an LLM platform. No
embedded knowledge bases, schemas (beyond wire frames above), or skill
loaders. The only "skill"-like assets are `pi-mure/` mirrored into
`internal/piext/assets/` for embedded install via `mure integration install pi`.

## Known Issues / Tech Debt

- No first-party `TODO`/`FIXME`/`HACK` comments in Go, TS, or shell sources.
- No tagged release yet (`pi-mure/package.json` is at `0.0.0`; README mentions
  `brew install` once a release is tagged).
- License is `TBD` in the top-level README.
- `<owner>` placeholder still in README install snippets and tmux plugin docs.
- `make sync-piext` is a manual step CI checks for drift; easy to forget when
  editing `pi-mure/`.

## Linear Progression

### 001 — init (`specs/001-init/`)

**Goal.** Define the three-deliverable monorepo (daemon+CLI, pi extension,
tmux plugin) and the two contracts (socket protocol, `@mure-*` options) that
let them evolve independently.
**Choices.** NDJSON over per-session Unix socket; daemon owns roster, plugin
owns tmux surfaces; neither side mutates the other.
**Result.** Established `cmd/mure`, `internal/daemon`, `internal/sock`,
`internal/sidebar`, `internal/tmuxctl`, `pi-mure/`, `tmux-mure/`, and the
verb set + frame catalog still in use today.

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

### The Arc

001 set the rule (two contracts, three pieces, nobody crosses lines).
002 polished the only user-facing TUI surface within its lane.
003 turned the daemon into a fan-out primitive by giving agents tools, still
without touching the wire.
004 made the multi-agent UX livable by quarantining subagent panes — and
proved the seam from 001 was real, because it stayed inside `cmd/mure`.

Each spec stays in exactly one layer. That is the project's discipline.
