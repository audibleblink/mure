# mure

> **mure** — Japanese 群れ, *"a herd, a flock, a swarm."* 

*See what your AI coding agents are doing — inside tmux, where you already work.*

`mure` is a tmux-native multiplexer for coding-agent panes. It watches every
agent you spawn and surfaces their live status — *working*, *blocked*,
*errored*, *idle* — right on the pane border, plus an optional sidebar with
the full roster. No web UI, no separate window manager, no new keybindings to
learn.


![](./docs/header.png)

## Why

Running several AI agents in parallel is normal now. The options today are:

- **Shell wrappers** — launch and disappear; you have no idea what any agent
  is up to without `Ctrl-b q`-ing every pane.
- **Custom multiplexers** — replace tmux entirely; throw out your config,
  plugins, and muscle memory.

mure is the third option: **your tmux, unchanged, plus a daemon that watches.**

## No projects. Just tmux sessions.

Most agent managers (Claude Squad, Conductor, sketch.dev, etc.) ship their
own notion of a *project*: a named container that owns a working directory,
an agent roster, layout, and lifecycle. You create projects in their UI,
switch between them in their UI, and your editor/terminal/tmux setup lives
separately — or gets swallowed entirely.

mure doesn't have projects. **A tmux session *is* the project.**

- The daemon is scoped per tmux session — one socket at
  `…/mure/<session>/daemon.sock`, one roster, one sidebar.
- The working directory is whatever the pane's shell is in. No metadata file,
  no "project root" registry.
- Switching projects = `tmux switch-client`. Listing them = `tmux ls`.
  Persistence = whatever you already use (tmux-resurrect, tmuxinator, a
  shell function, nothing at all).
- Tearing down a project = `tmux kill-session`. The daemon goes with it.

If you already organise work as one-tmux-session-per-repo, mure slots in
without asking you to learn a second hierarchy.

## Where new agent panes land

When something (you, or another agent via `mure_spawn`) spawns an agent,
mure has to pick *where* the new pane appears. That's controlled by
`@mure-spawn-target`. The value is either the reserved keyword
`subagents-window` *(default)* — which triggers find-or-create of a
dedicated window — or **any tmux command** that creates a pane. mure
appends `-P -F '#{pane_id}' <agent-command>` and runs it.

The plugin ships these named presets (set them *before* sourcing the
plugin); they're rewritten to plain tmux commands at load time:

| Preset | Expands to | Behaviour |
|---|---|---|
| `subagents-window` *(default)* | — (special) | Find a window tagged `@mure-subagents-window=1` in this session and split it; otherwise create one named `subagents` in the background. Keeps agents out of the window you're working in. |
| `right-of-active` | `split-window -h` | New column next to the active pane. |
| `below-active` | `split-window -v` | New row below the active pane. |
| `new-window` | `new-window` | One agent per window, foregrounded. |

Because the value is just a tmux command, you can write your own:

```tmux
set -g @mure-spawn-target "split-window -h -f"            # full-height right column
set -g @mure-spawn-target "split-window -v -f -l 30%"     # bottom strip, 30% tall
set -g @mure-spawn-target "new-window -t :9"              # pin to window index 9
```

All behavior definitions live in the plugin (`tmux-mure/tmux-mure.tmux`),
so users can override or extend them without touching Go.

## Install

Requires **tmux ≥ 3.2** and (from source) **Go ≥ 1.24**.

```sh
# from source
git clone https://github.com/<owner>/mure
cd mure
make build          # → ./bin/mure, then move it onto your $PATH
```

Then install the tmux plugin. With [TPM](https://github.com/tmux-plugins/tpm):

```tmux
set -g @plugin '<owner>/mure'
run '~/.tmux/plugins/tpm/tpm'
```

…or source it directly from a local checkout:

```tmux
run-shell /absolute/path/to/mure/tmux-mure/tmux-mure.tmux
```

Reload tmux (`prefix + I` for TPM, or `tmux source-file ~/.tmux.conf`).

If you drive agents with [`pi`](https://github.com/audibleblink/pi), install
the extension that teaches them to report in:

```sh
mure integration install pi
```

Check everything's wired:

```sh
mure doctor
```

## Use it

```sh
mure up                       # start the daemon for this tmux session
mure spawn worker "build X"   # open a pane running an agent
mure ls                       # list agents (add --json for scripts)
mure sidebar                  # open the live roster (or prefix-M)
mure focus <agent>            # jump to that agent's pane
mure wait <agent>             # block until the agent emits its final result
mure down                     # stop the daemon
```

Agent state shows up two places automatically:

- **On the pane border** — color + `[status]` tag next to the title.
- **In the sidebar** — toggle with `prefix + M`.

### Agents that orchestrate other agents

When `mure` drives `pi`, agents get two extra tools:

- `mure_spawn` — fan out a sibling agent in a new pane.
- `mure_wait` — block on its result.

Useful for "planner spawns five workers, waits for all of them" workflows.
The tools only appear inside mure-managed panes, so they're invisible
elsewhere.

## Customize

A few tmux options you might care about. Set them **before** the `run-shell` /
TPM `run` line.

```tmux
set -g @mure-sidebar-width    36
set -g @mure-sidebar-position left           # left | right | top | bottom
set -g @mure-sidebar-key      M              # prefix-key for sidebar toggle
set -g @mure-spawn-target     subagents-window
#                             ^ keyword `subagents-window` or any tmux
#                               pane-creating command (see above)

# per-status colors on the pane border
set -g @mure-color-working      green
set -g @mure-color-blocked      yellow
set -g @mure-color-errored      red
set -g @mure-color-disconnected colour244
set -g @mure-color-idle         colour250
```

Full list and `status-right` snippet: [`tmux-mure/README.md`](./tmux-mure/README.md).

## What's in the box

| Piece | What it does |
|---|---|
| `mure` daemon + CLI | One small Go binary. The only thing you install. |
| Sidebar TUI | Bubble Tea pane (`mure sidebar`), opens via `prefix + M`. |
| `tmux-mure` plugin | Pure-shell tmux plugin: hooks, border format, sidebar toggle. |
| `pi-mure` extension | Optional. Makes `pi` agents report status into the daemon. |

The daemon talks NDJSON over a per-session Unix socket
(`~/Library/Caches/mure/<session>/daemon.sock` on macOS,
`$XDG_RUNTIME_DIR/mure/<session>/` on Linux, mode `0700`). Nothing leaves
your machine.

## Development

```sh
make build         # sync pi-ext mirror + build ./bin/mure
make test          # go test ./... + shellcheck
make tmux-test     # real-tmux hook integration test
make verify        # everything, including lint
```

Design notes and spec history live under [`specs/`](./specs/); the current
shape of the codebase is in [`specs/current.md`](./specs/current.md).

## License

TBD.
