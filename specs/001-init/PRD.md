# mure — Product Requirements Document

A monorepo containing three coupled deliverables:

1. **`mure`** — Go daemon + CLI + Bubble Tea sidebar.
2. **`pi-mure`** — TypeScript pi extension that emits agent state into the daemon.
3. **`tmux-mure`** — TPM-installable tmux plugin owning all tmux-side config (hooks, status-line, decoration, sidebar layout, spawn policy).

The three meet at two stable contracts:

- **Socket protocol** (NDJSON over Unix domain socket) — daemon ↔ pi extension ↔ `mure _hook`.
- **`@mure-*` pane options** — written by daemon, read by tmux-mure for display.

Neither side mutates the other's surface. The daemon installs no tmux config.
The plugin starts no processes.

Name: Japanese 群れ — "herd, flock, swarm".

---

## 1. Problem

Running multiple AI coding agents in parallel is now normal. Existing options:

- **splitmind / shell orchestrators** — stateless; launch and disappear; no observability.
- **herdr** — replaces tmux with a custom multiplexer; abandons existing config, plugins, muscle memory.

mure is the third option: **tmux unchanged, plus a daemon that watches.**

Prior art:
- https://github.com/jerdna-regeiz/splitmind
- https://herdr.dev/ · https://github.com/ogulcancelik/herdr
- https://github.com/ogulcancelik/herdr/blob/master/src/integration/assets/pi/herdr-agent-state.ts

---

## 2. Goals

1. Spawn N parallel coding agents in a single tmux session.
2. Surface live, semantic agent state (working / blocked / idle / errored) without scraping pane output.
3. Treat tmux as the display surface; never intercept tmux keybindings.
4. XDG-compliant paths on Linux; explicit `~/Library/Caches/mure/` on macOS.
5. Daemon owns no tmux config; plugin owns no processes; pi extension is a no-op outside mure.

## 3. Non-goals

1. Not a tmux replacement.
2. Not a TUI for navigation. tmux owns pane movement.
3. Not a session manager.
4. Not a git tool. Worktrees/branches/merges are the user's responsibility.
5. Not resumable across daemon restart with persisted history — but agents auto-reconnect, so a fresh daemon repopulates the roster from incoming `hello` frames.
6. Not agent-agnostic at v1. pi only; others in v2.
7. Not multi-host.
8. No web dashboard, no Electron.

---

## 4. Repository layout

```
mure/                          # repo root; single Go module
├── PRD.md
├── README.md
├── go.mod
├── go.sum
├── Makefile                   # orchestrates Go build + asset sync + plugin lint
├── cmd/mure/                  # CLI entrypoint
├── internal/
│   ├── daemon/                # control-mode + socket server
│   ├── tmuxctl/               # tmux control-mode protocol
│   ├── sock/                  # NDJSON framing, types
│   ├── sidebar/               # Bubble Tea sidebar
│   └── piext/                 # go:embed wrapper
│       └── assets/            # synced from ../../pi-mure/ at build time
├── pi-mure/                   # TypeScript pi extension (source of truth)
│   ├── package.json
│   ├── tsconfig.json
│   ├── index.ts
│   └── test/
├── tmux-mure/                 # TPM-installable tmux plugin (pure shell + tmux)
│   ├── tmux-mure.tmux         # TPM entrypoint
│   ├── scripts/
│   │   ├── sidebar-toggle.sh
│   │   └── uninstall-hooks.sh
│   ├── example.tmux.conf      # recommended user snippet
│   └── README.md
└── .github/workflows/
```

**Embed path note.** Go's `//go:embed` cannot traverse `..`, so `internal/piext/assets/` is a build-time sync target. The Makefile target `internal/piext/assets: pi-mure/` runs `rsync -a --delete pi-mure/ internal/piext/assets/` before `go build`. CI verifies the sync was committed; local devs working on the extension run `make sync-piext`.

**Why one Go module, not three.** The daemon, CLI, and sidebar share types from `internal/sock/`. Splitting into multiple modules would require either a publishable shared package or duplicated types. Single module, internal packages, one binary. The pi extension and tmux plugin are not Go and do not participate in the module.

**Tagging.** Each subproject is released independently:
- `mure-v0.1.0` — daemon/CLI binary tag.
- `pi-mure-v0.1.0` — npm package tag (if published) / extension version surfaced in `hello` frames.
- `tmux-mure-v0.1.0` — TPM consumers can pin.

Compatibility is governed by the protocol version (`"v":1`), not these tags.

---

## 5. Architecture

### 5.1 Process topology

```
┌─────────────────────── tmux server (unchanged) ─────────────────────────┐
│   session: mure-<project>                                               │
│   ┌─ sidebar ──┐  ┌─ agent 1 ─┐  ┌─ agent 2 ─┐                          │
│   │ mure       │  │ pi + ext  │  │ pi + ext  │                          │
│   └─────┬──────┘  └────┬──────┘  └────┬──────┘                          │
└─────────┼──────────────┼──────────────┼─────────────────────────────────┘
          │              │              │
          ▼              ▼              ▼
       ┌──────────────────────────────────┐
       │  mure daemon (Go)                │
       │  • tmux -C reader + writer       │
       │  • Unix socket server            │
       │  • in-memory roster              │
       │  • @mure-* pane-option mirror    │
       └──────────────────────────────────┘
                       ▲
                       │ hooks invoke `mure _hook …`
                       │
           ┌───────────────────────────┐
           │   tmux-mure plugin        │
           │   • hooks                 │
           │   • status-line format    │
           │   • sidebar key bindings  │
           │   • spawn target policy   │
           └───────────────────────────┘
```

### 5.2 State ownership

- Agents own their status.
- Daemon aggregates **in memory**.
- Pane options (`@mure-*`) are a coarse external mirror, updated ≤2 Hz.
- Sidebar reads from the daemon's socket, not from pane options.

If the daemon dies, the in-memory roster is lost. Agents detect EPIPE and reconnect with backoff (cap 30s); a freshly-started daemon repopulates its roster purely from incoming `hello` frames, which carry `pane_id`. **No persistence layer. Auto-rediscovery is the recovery story.**

### 5.3 Failure semantics

| Failure | User-visible impact |
|---|---|
| Daemon crashes | Sidebar shows `(disconnected)`. Agents reconnect-loop. On `mure up`, roster rebuilds within ~30s. |
| Sidebar pane killed | Daemon and agents unaffected. Re-toggle via `tmux-mure`. |
| Agent crashes | Daemon sees EPIPE → debounce 1s → either `errored` (if `%pane-died` arrives) or `disconnected`. |
| tmux server killed | Everything dies. Intentional. |
| Plugin missing | `mure up` warns; focus tracking and status-line don't work; daemon and sidebar still function. |
| Daemon missing | Plugin hooks no-op via `command -v mure` guard. |

---

## 6. Daemon

**Language:** Go 1.22+.

### 6.1 Responsibilities

1. Open two `tmux -C` connections to the target session:
   - **Reader**: issues `refresh-client -f no-output` so `%output` frames are suppressed; parses `%window-add`, `%window-close`, `%pane-died`, `%session-window-changed`, `%layout-change`.
   - **Writer**: issues commands serially, correlating responses via `%begin`/`%end`. No pipelining.

   The split is for parser cleanliness (async `%` notifications versus synchronous command replies), not for throughput.

2. Listen on `$MURE_RUN_DIR/daemon.sock`.
3. Translate inbound `status` frames into pane-option writes, coalesced 500ms per `(pane_id, option)`.
4. Maintain the in-memory roster; push updates to sidebar clients.

### 6.2 Concurrency model

One goroutine per socket connection; one reader; one writer (the coalescer); one roster owner. All roster mutations serialize through the owner via a channel.

### 6.3 EPIPE vs. pane-died race

On EPIPE, the daemon withholds visible state change for 1s. If `%pane-died` arrives in that window, the pane goes to `errored`. Otherwise `disconnected`. During the debounce, the sidebar continues to render the previous status (no flicker). If reader lag exceeds 1s, the window stretches to match.

### 6.4 Stale-socket cleanup

On startup, if `daemon.sock` exists, the daemon attempts a `hello` ping. No response within 200ms → unlink and proceed. PID file (`daemon.pid`) is advisory only; never trusted alone.

### 6.5 Re-entrancy

`mure up` against a healthy existing daemon exits 0 with `"already running"`. Detection: ping the socket.

### 6.6 Logging

`$MURE_RUN_DIR/daemon.log`, rotated at 4MB with one `.1` sibling.

### 6.7 No tmux config mutation

The daemon installs no hooks, sets no global options, writes no key bindings. It only writes `@mure-*` pane options on panes it knows about.

---

## 7. CLI

| Verb | Description |
|---|---|
| `mure up` | Start daemon; open control-mode clients. Re-entrant. Warns if `tmux-mure` plugin not detected. |
| `mure spawn <role> [task]` | Open a new pane via `tmux split-window` (target read from `@mure-spawn-target`; default `subagents-window`) and exec `$MURE_AGENT_CMD` (default `pi`). Sets `MURE_ENV=1`, `MURE_AGENT_ID`, `MURE_SOCKET`, and `@mure-role`, `@mure-spawned-at` on the new pane. |
| `mure ls` | Human-readable table. `--json` for machine output. |
| `mure focus <agent>` | Exec `tmux select-pane`. |
| `mure down` | Stop daemon, unlink socket. tmux panes persist. |
| `mure sidebar` | (Internal) Bubble Tea sidebar. Invoked by `tmux-mure` toggle script. Does not split windows. |
| `mure integration install pi` | Write embedded pi extension to `$PI_CODING_AGENT_DIR/extensions/mure/`. |
| `mure integration uninstall pi` | Remove it. |
| `mure _hook <event> <args…>` | (Internal) Called by `tmux-mure` hook scripts. Opens socket, writes one line, exits. |
| `mure doctor` | Check tmux ≥3.2, plugin presence, socket permissions, peer-auth syscall availability. |

---

## 8. State model

**Source of truth for sidebar:** daemon's in-memory roster, pushed over socket.

**External mirror (≤2 Hz):** `@mure-*` pane user-options.

### 8.1 Session environment (set once at `mure up`)

| Variable | Meaning |
|---|---|
| `MURE_RUN_DIR` | Per-session run directory. |
| `MURE_SOCKET` | Daemon socket path. |
| `MURE_SESSION` | tmux session name. |

### 8.2 Pane user options

| Option | Owner | Meaning |
|---|---|---|
| `@mure-agent-id` | daemon | Pane-lifetime agent identifier (survives daemon restart). |
| `@mure-role` | daemon (at spawn) | Free-form label. |
| `@mure-spawned-at` | daemon | Unix ms. |
| `@mure-status` | daemon | `idle` \| `working` \| `blocked` \| `disconnected` \| `errored`. |
| `@mure-task` | daemon | Short string. Server-side truncated to 256 bytes. |
| `@mure-last-turn-ended-at` | daemon | Unix ms or empty. |
| `@mure-is-sidebar` | plugin | Set by `sidebar-toggle.sh`. |

### 8.3 Plugin-readable options (user-settable)

| Option | Default | Meaning |
|---|---|---|
| `@mure-sidebar-width` | `36` | Sidebar pane width. |
| `@mure-sidebar-position` | `left` | `left` \| `right`. |
| `@mure-sidebar-key` | `M` | Prefix-key for sidebar toggle. |
| `@mure-spawn-target` | `subagents-window` | `subagents-window` \| `right-of-active` \| `below-active` \| `new-window`. Unknown values warn and fall back to `subagents-window`. |
| `@mure-color-{working,blocked,errored,disconnected,idle}` | (see plugin) | Pane border colors. |
| `@mure-status-format` | (see plugin) | Status-line format snippet. |
| `@mure-plugin-version` | `1` | Set by plugin; checked by `mure doctor`. |

Pane titles are **not** written by the daemon. The plugin sets a title format that reads `@mure-status` if desired.

---

## 9. Sidebar

**Implementation:** Bubble Tea, invoked as `mure sidebar`. The only Charm surface in mure.

**Render (illustrative; final glyph set TBD):**

```
┌─ mure ─────────────────────┐
│ project-x · 6 agents       │
│                            │
│ ● agent-1   working   2:14 │
│ ◐ agent-2   blocked   0:31 │
│ ✓ agent-3   idle      4:08 │
│ ○ agent-4   idle      —    │
│ ⚠ agent-5   errored   1:02 │
│ ⋯ agent-6   discon.   0:08 │
└────────────────────────────┘
```

**State:** Pure view. Connects to `$MURE_SOCKET`, subscribes to roster updates. On disconnect, displays `(disconnected)` and reconnects with backoff capped at 30s.

**Backpressure:** daemon's per-sidebar send buffer is bounded (64 frames). Overflow → daemon closes the connection; sidebar reconnects and receives a fresh `roster` frame.

**Keybindings (sidebar pane only):**

| Key | Action |
|---|---|
| `j` / `k` | Move selection. |
| `↵` | `tmux select-pane -t <pane>`. |
| `q` | `tmux kill-pane -t $TMUX_PANE`. |

Socket connection is **read-only**. No mutating frames from sidebar.

Layout (position, width, toggle key) is owned by `tmux-mure`. `mure sidebar` does not split windows; it expects to be exec'd inside an already-split pane.

---

## 10. pi extension (`pi-mure`)

**Install:** `mure integration install pi` writes embedded files (from `internal/piext/assets/`, synced from `pi-mure/`) to:

```
$PI_CODING_AGENT_DIR/extensions/mure/
├── package.json
└── index.ts
```

`PI_CODING_AGENT_DIR` defaults to `~/.pi/agent`. Installer is hermetic and offline.

**Extension behavior:**

1. **Gate** — return immediately unless both `MURE_ENV=1` and `MURE_AGENT_ID` are set.
2. **Connect** — open `MURE_SOCKET`; send `hello` with `agent_id`, `pane_id` (from `$TMUX_PANE`), `pid`, `pi_version`.
3. **Subscribe** to pi lifecycle events:
   - `session_start` → `status: idle`
   - `agent_start` → `status: working` with cached task text
   - `tool_execution_start` → `status: working` with `tool`
   - `tool_call` awaiting confirmation → `status: blocked`
   - `agent_end` → `status: idle` with `last_turn_ended_at = now`
   - `session_shutdown` → `bye`, close socket
4. **Reconnect** with exponential backoff 250ms → 30s, jittered.
5. **Outbound buffering** during disconnect: coalesce by event type (new `status` replaces buffered `status`; `hello`/`bye` never drop).

Hot-reload semantics unverified; treat reinstall as requiring session restart until confirmed otherwise with pi.

**v2:** Mirror `@ogulcancelik/pi-herdr` by registering mure tools as pi tools (`mure_spawn`, `mure_wait_agent`, `mure_focus`). Same socket, opposite direction.

---

## 11. tmux-mure plugin

A TPM-installable plugin that owns all tmux-side surfaces. Pure shell + tmux config; zero Go, zero TS.

### 11.1 Hooks

Installed at plugin-load (`tmux-mure.tmux` invoked by TPM):

```tmux
set-hook -g after-select-pane \
  'run-shell -b "command -v mure >/dev/null && mure _hook focus #{pane_id} #{client_name} || true"'

set-hook -g pane-exited \
  'run-shell -b "command -v mure >/dev/null && mure _hook pane_died #{pane_id} || true"'

set-hook -g session-closed \
  'run-shell -b "command -v mure >/dev/null && mure _hook session_closed #{hook_session} || true"'
```

**Substitution timing.** tmux substitutes `#{pane_id}` etc. at hook *fire* time. Verified against tmux 3.2+.

**Fork cost.** Each focus change forks `mure _hook`. Users with rapid pane-switching workflows can replace the hook body with a shell function piping to `socat - UNIX-CONNECT:$MURE_SOCKET`. The plugin ships the simple version.

**Idempotency.** Re-sourcing replaces global hooks atomically.

### 11.2 Status-line snippet

```tmux
set -g @mure-status-format '#{?#{@mure-status},[#{@mure-status}] ,}'
```

User appends manually:

```tmux
set -g status-right '#{@mure-status-format}%H:%M'
```

The plugin does **not** rewrite `status-right`. Hostile to overwrite user config.

### 11.3 Pane decoration

```tmux
set -hg pane-border-format \
  '#{?#{@mure-status},#[fg=#{@mure-status-color}]#{@mure-status} #{@mure-task},}'
```

`@mure-status-color` derived from `@mure-status` via a `#{?…}` format chain shipped by the plugin, sourcing the per-status `@mure-color-*` options.

### 11.4 Sidebar toggle

`bind-key -T prefix M run-shell 'tmux-mure-sidebar-toggle'`

Script logic:
1. If a pane with `@mure-is-sidebar=1` exists in the current session → `kill-pane` it.
2. Else → `split-window -hb -l "$(tmux show-option -gv @mure-sidebar-width)" 'mure sidebar'`, then `set -p @mure-is-sidebar 1`.

`-hb` flips based on `@mure-sidebar-position`.

### 11.5 Spawn-target policy

`mure spawn` reads `@mure-spawn-target` via `tmux show-option -gv`. If unset (plugin not installed), defaults to `subagents-window`.

### 11.6 Installation

```tmux
# ~/.tmux.conf
set -g @plugin 'alex/mure'        # TPM points at this monorepo; subpath 'tmux-mure'
run '~/.tmux/plugins/tpm/tpm'
```

(TPM supports subpath via convention or symlink; the plugin's release tag is `tmux-mure-vX.Y.Z`.)

`mure doctor` probes `tmux show-option -gv @mure-plugin-version`. Missing → prints install line. Mismatched major → warns.

### 11.7 Uninstall

TPM-standard removes files. Hooks remain registered until tmux server restart; `scripts/uninstall-hooks.sh` runs the `set-hook -gu` lines.

---

## 12. Socket protocol

**Transport:** Unix domain socket at `$MURE_RUN_DIR/daemon.sock`. Parent dir mode `0700`, socket mode `0600`, set explicitly.

**Peer auth:** filesystem mode is primary defense. Belt-and-suspenders: on `accept`, check peer UID via `SO_PEERCRED` (Linux) / `getpeereid(3)` via `golang.org/x/sys/unix` (macOS). Reject non-self UIDs. FreeBSD not supported in v1.

**Connection roles:** first frame must be `hello` with `role`: `"agent"` | `"sidebar"` | `"cli"` | `"hook"`. Sidebar and hook connections are non-mutating. Agents may only send frames pertaining to themselves.

**Framing:** newline-delimited JSON. Max frame 64KB. Oversize → close.

**Versioning:** every frame includes `"v": 1`. v1 daemons reject any other version. Negotiation deferred to v2.

### 12.1 Agent → daemon

```jsonc
{"v":1,"event":"hello","role":"agent","agent_id":"agent-3","pane_id":"%41","pid":12345,"pi_version":"0.50.3","ts":1731890000000}
{"v":1,"event":"status","agent_id":"agent-3","status":"working","task":"refactor auth","tool":"bash","ts":1731890001234}
{"v":1,"event":"bye","agent_id":"agent-3","ts":1731890003000}
```

On reconnect, agents re-send `hello` plus current `status`. Daemon rebuilds roster purely from incoming `hello` frames — no pane-option scan.

### 12.2 Hook → daemon (from `tmux-mure`)

```jsonc
{"v":1,"event":"hello","role":"hook"}
{"v":1,"event":"focus","pane_id":"%41","client":"main","ts":1731890004000}
{"v":1,"event":"pane_died","pane_id":"%41"}
{"v":1,"event":"session_closed","session":"mure-project-x"}
```

Daemon dedupes `focus` events by `(pane_id, ts)` within 50ms (multi-client tmux).

### 12.3 Daemon → sidebar

```jsonc
{"v":1,"event":"roster","agents":[…]}
{"v":1,"event":"agent_update","agent":{"id":"agent-3","status":"idle","task":"…","pane":"%43","last_turn_ended_at":1731890003000}}
```

### 12.4 Daemon → agent

```jsonc
{"v":1,"event":"focus","focused":true,"ts":1731890004000}
```

### 12.5 Status vocabulary

| Status | Meaning |
|---|---|
| `idle` | Agent alive, not working a turn. `last_turn_ended_at` distinguishes "just finished" from "never worked". |
| `working` | Mid-turn. |
| `blocked` | Awaiting user input. |
| `disconnected` | Socket dead; pane process may be alive. Reached after 1s EPIPE debounce. |
| `errored` | `%pane-died` received, or explicit error frame. |

No `done` state. "Turn just finished" = `idle` with recent `last_turn_ended_at`. "User has acknowledged" = a `focus` event on the pane after `last_turn_ended_at`; daemon then clears `last_turn_ended_at`. Drives sidebar's `✓` glyph.

---

## 13. Testing

- `tmuxctl.Client` interface; unit tests use a scripted fake replaying canned `%` event streams.
- Integration tests spawn real `tmux` in a temp `TMUX_TMPDIR`; stub agent speaks the protocol.
- Throughput test: ≥8 panes streaming `yes`; assert daemon status-write p99 < 100ms with `%output` suppressed.
- `pi-mure/test/` — TS suite mocking the socket; asserts frame shapes.
- `tmux-mure/` lints via `shellcheck` on scripts; hooks tested via a dockerized tmux server fixture.
- CI: darwin + linux.

---

## 14. Tech stack

| Layer | Choice |
|---|---|
| Daemon + CLI | Go 1.22+ |
| Sidebar TUI | Bubble Tea + Lipgloss + Bubbles |
| Embedded assets | `//go:embed` |
| pi extension | TypeScript |
| tmux plugin | POSIX shell + tmux config |
| Distribution | Homebrew tap (binary); TPM (plugin) |

**Minimum versions:**

| Dependency | Minimum | Reason |
|---|---|---|
| tmux | 3.2 | `refresh-client -f`, per-pane user options. Verified at `mure doctor`. |
| Go | 1.22 | `//go:embed`, modern stdlib. |
| macOS | 11+ | `getpeereid` semantics. |
| Linux | any with `SO_PEERCRED` | Effectively all. |

**Not used:** SQLite, libtmux, zellij, Electron.

---

## 15. Runtime layout

```
Linux:   $XDG_RUNTIME_DIR/mure/<session>/
macOS:   ~/Library/Caches/mure/<session>/

  daemon.sock      # Unix socket (mode 0600)
  daemon.pid       # advisory
  daemon.log       # rotated at 4MB, one .1
```

---

## 16. Environment variables

| Variable | Set by | Consumed by | Purpose |
|---|---|---|---|
| `MURE_ENV` | daemon (per pane) | pi extension | Activation gate. |
| `MURE_SESSION` | daemon | all | tmux session name. |
| `MURE_RUN_DIR` | daemon | all | Per-session run dir. |
| `MURE_SOCKET` | daemon | agents, hook | Daemon socket path. |
| `MURE_AGENT_ID` | daemon (per pane) | pi extension | Pane-lifetime agent identifier. |
| `MURE_AGENT_CMD` | user (optional) | `mure spawn` | Defaults to `pi`. |
| `PI_CODING_AGENT_DIR` | user (optional) | `mure integration install pi` | Pi config dir override. |

---

## 17. Build & distribution

- `make build` → `make sync-piext` → `go build -o mure ./cmd/mure`.
- CI builds darwin/amd64, darwin/arm64, linux/amd64, linux/arm64.
- Homebrew tap auto-updated on `mure-vX.Y.Z` tag.
- TPM consumers reference the repo with subpath `tmux-mure/`; pinned via `tmux-mure-vX.Y.Z` tags.
- No telemetry, no update-checker, no analytics.

---


## 18. Reference prior art

- **herdr** — https://herdr.dev/ · https://github.com/ogulcancelik/herdr
- **splitmind** — https://github.com/jerdna-regeiz/splitmind
- **pi** — https://pi.dev/ · https://github.com/badlogic/pi-mono
- **@ogulcancelik/pi-herdr** — https://github.com/ogulcancelik/pi-extensions/blob/main/packages/pi-herdr
- **tmux control mode** — `man tmux`, section CONTROL MODE.
- **Bubble Tea** — https://github.com/charmbracelet/bubbletea
