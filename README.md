# mure

`mure` is a tmux-native multiplexer for **coding-agent panes**. It surfaces live
per-agent status (working / blocked / errored / idle) inside an ordinary tmux
session — no separate window manager, no web UI.

The repo ships four pieces that work together:

| Piece | Path | What it does |
|---|---|---|
| **`mure` daemon + CLI** | `cmd/mure`, `internal/daemon` | Go binary. Owns the per-session Unix socket, the agent roster, and tmux bookkeeping. |
| **Sidebar TUI** | `internal/sidebar` | Bubble Tea pane (`mure sidebar`) showing the live roster. |
| **`pi-mure` extension** | `pi-mure/` | TypeScript [pi](https://github.com/audibleblink/pi) extension that emits agent state into the daemon. |
| **`tmux-mure` plugin** | `tmux-mure/` | Pure-shell tmux plugin: hooks, `pane-border-format`, sidebar toggle. |


## How it works

1. The `tmux-mure` plugin installs tmux hooks that fork `mure _hook …` on pane
   focus / exit / session-close.
2. `mure up` starts the daemon for the current tmux session. It listens on a
   Unix socket under the per-session runtime dir and attaches to tmux in
   control-mode.
3. Coding agents (driven by `pi-mure`) emit framed state messages over the
   socket. The daemon coalesces them, updates pane options
   (`@mure-status`, `@mure-task`), and pushes events to any connected
   sidebar clients.
4. `mure sidebar` (or prefix-`M` via the plugin) opens a Bubble Tea pane that
   subscribes to the same socket.

## Install

### Daemon / CLI

Homebrew (once a release is tagged):

```sh
brew install <owner>/tap/mure
```

From source:

```sh
git clone https://github.com/<owner>/mure
cd mure
make build           # → ./bin/mure
```

Requires Go ≥ 1.22 and tmux ≥ 3.3.

### tmux plugin

From a local checkout/directory, source the plugin file directly:

```tmux
# set options first, if desired
set -g @mure-sidebar-width 36
set -g @mure-sidebar-position left

run-shell /absolute/path/to/mure/tmux-mure/tmux-mure.tmux
```

Reload tmux config:

```sh
tmux source-file ~/.tmux.conf
```

With TPM:

```tmux
set -g @plugin '<owner>/mure'      # tmux-mure lives under tmux-mure/
run '~/.tmux/plugins/tpm/tpm'
```

Then `prefix + I`. See [`tmux-mure/README.md`](./tmux-mure/README.md) and
[`tmux-mure/example.tmux.conf`](./tmux-mure/example.tmux.conf).

### pi extension

```sh
cd pi-mure
npm install
pi extension install .
```

Or let `mure` do it: `mure integration install pi`.

## Quick start

```sh
mure up                       # start the daemon for $TMUX session
mure spawn worker "build X"   # open a pane running an agent
mure ls                       # list agents (add --json for scripts)
mure sidebar                  # open the live roster TUI
mure focus <agent>            # select that agent's pane
mure doctor                   # diagnostics (plugin, socket, tmux version)
mure down                     # stop the daemon
```

## Configuration

### Environment variables

| Var | Default | Meaning |
|---|---|---|
| `MURE_SESSION` | tmux `#S`, else `default` | Session name; namespaces the runtime dir + socket. |
| `MURE_SOCKET` | `<runDir>/daemon.sock` | Override the Unix socket path. |
| `MURE_RUN_DIR` | see below | Override the per-session runtime dir. |
| `MURE_TMUX_SOCKET` | parsed from `$TMUX` | tmux server socket the daemon attaches to. |
| `MURE_AGENT_ID` | — | Set by `mure spawn` inside the agent pane. |
| `MURE_AGENT_CMD` | — | Command `mure spawn` exec's as the agent. |
| `MURE_TASK` | — | Initial task label for the spawned agent. |
| `MURE_ENV` | — | Comma-separated extra env passed into spawned panes. |
| `MURE_DAEMON` | unset | Internal — set on the forked daemon process. |

### Runtime directory

| OS | Path |
|---|---|
| macOS | `~/Library/Caches/mure/<session>/` |
| Linux | `$XDG_RUNTIME_DIR/mure/<session>/` (fallback `/tmp/mure-<uid>/<session>/`) |

Permissions are forced to `0700`. The socket lives at `<runDir>/daemon.sock`.

### tmux plugin options

Set before TPM's `run` line. Defaults:

| Option | Default |
|---|---|
| `@mure-sidebar-width` | `36` |
| `@mure-sidebar-position` | `left` |
| `@mure-sidebar-key` | `M` |
| `@mure-spawn-target` | `subagents-window` |
| `@mure-color-working` | `green` |
| `@mure-color-blocked` | `yellow` |
| `@mure-color-errored` | `red` |
| `@mure-color-disconnected` | `colour244` |
| `@mure-color-idle` | `colour250` |
| `@mure-status-format` | `#{?#{@mure-status},[#{@mure-status}] ,}` |

Full table in [`tmux-mure/README.md`](./tmux-mure/README.md).

## Development

```sh
make build         # sync pi-ext mirror + build ./bin/mure
make test          # go test ./... + shellcheck
make tmux-test     # tmux hook integration test
make verify        # everything above + lint
```

`make sync-piext` mirrors `pi-mure/` into `internal/piext/assets/` for the
embedded copy; CI fails if the mirror is dirty.

## Release flow

Three independently-versioned tag prefixes, pushed in this order:

1. **`mure-vX.Y.Z`** — Go daemon + CLI. Triggers
   `.github/workflows/release.yml` → GoReleaser → `darwin/{amd64,arm64}` +
   `linux/{amd64,arm64}` archives + checksums, and (with `HOMEBREW_TAP_TOKEN`)
   a Homebrew formula update.
2. **`pi-mure-vX.Y.Z`** — pi extension. Bump `pi-mure/package.json`, run
   `make sync-piext`, commit, tag, publish from `pi-mure/`.
3. **`tmux-mure-vX.Y.Z`** — tmux plugin (shell only). Just tag.

Rehearse locally:

```sh
goreleaser release --snapshot --clean
```

## License

TBD.
