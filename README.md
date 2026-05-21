# mure

> **mure** _mɯ̟́ɾè̞_ — Japanese 群れ, *"a herd, a flock, a swarm."* 

*See what your AI coding agents are doing — inside tmux, where you already work.*

`mure` is a tmux-native multiplexer for coding-agent panes. It watches every
agent you spawn and surfaces their live status — *working*, *blocked*,
*idle* — visible via `mure ls` and an optional sidebar with the full roster.

No web UI, no separate window manager, no new keybindings to learn.


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

Teach a coding-agent harness to report into mure:

```sh
mure integration list                  # show available harnesses
mure integration install pi             # or: claude, opencode
```

See [Adding a harness](#adding-a-harness) below to wire up a new one.

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

See agent state via:

- **`mure ls`** (or `mure ls --json`) — current roster.
- **The sidebar** — toggle with `prefix + M`.

### Agents that orchestrate other agents

When `mure` drives `pi`, agents get two extra tools on harnesses that allow custom tools:

- `mure_spawn` — fan out a sibling agent in a new pane.
- `mure_wait` — block on its result.

Useful for "planner spawns five workers, waits for all of them" workflows.
The tools only appear inside mure-managed panes, so they're invisible
elsewhere.

Mure will use Skills as a fallback

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
```

Full list: [`tmux-mure/README.md`](./tmux-mure/README.md).

## What's in the box

| Piece | What it does |
|---|---|
| `mure` daemon + CLI | One small Go binary. The only thing you install. |
| Sidebar TUI | Bubble Tea pane (`mure sidebar`), opens via `prefix + M`. |
| `tmux-mure` plugin | Pure-shell tmux plugin: hooks, sidebar toggle, spawn-target. |
| Harness manifests | `harnesses/<name>/` ships a manifest + skill + installable files (hooks, plugins) for each supported coding agent (`pi`, `claude`, `opencode`). |

The daemon talks NDJSON over a per-session Unix socket
(`~/Library/Caches/mure/<session>/daemon.sock` on macOS,
`$XDG_RUNTIME_DIR/mure/<session>/` on Linux, mode `0700`). Nothing leaves
your machine.

## Adding a harness

A *harness* is a coding-agent CLI mure knows how to launch and listen to.
Each one is a directory under [`harnesses/`](./harnesses) with a single
`manifest.toml`, an optional `SKILL.md` (Agent Skills spec — YAML frontmatter with `name` and `description`), and any files (shell hooks, TS
plugins, config snippets) the harness needs dropped onto disk.

1. **Create the folder.** `harnesses/<name>/` — `<name>` is what users type
   in `mure integration install <name>` and `mure spawn --harness <name>`.
2. **Write `manifest.toml`.** Required keys: `manifest_version`, `name`,
   `command`, `task_arg` (`positional` | `stdin` | `flag:--xyz` | `none`).
   Declare `[capabilities]` honestly — `status` and `result` should be
   `true` only if the harness emits NDJSON frames via a hook script or a
   plugin (see below). Declaring `status=false` or `result=false` is
   reserved for a future `tmux capture-pane` fallback that is **not yet
   implemented**; today such a harness will emit no status or result
   frames at all. `mure integration list` labels these `degraded`.
3. **Drop the bridge.** Whatever your harness uses to signal tool/turn
   lifecycle — shell hooks (claude), a TS plugin (pi, opencode) — each
   one ultimately produces a `mure emit status …` or `mure emit result -`
   frame. List the files under `[[install.files]]` with `src` (path
   inside the harness folder), `dst` (target, `~` expanded), and `mode`.
   Shell hooks use `0755`; plugins / data files use `0644`.

   Shell hooks must guard against `mure` being absent from the hook's
   `$PATH` (the harness can be invoked outside a mure session). Prefix
   each script with `command -v mure >/dev/null 2>&1 || exit 0`.
4. **Optional skill file.** `SKILL.md` is the instruction blob that
   teaches the agent that `mure spawn` / `mure wait` exist. It must
   begin with YAML frontmatter (`name`, `description`) per the Agent
   Skills spec, and is conventionally installed into a `skills/<name>/`
   directory. Declare its destination and merge strategy under
   `[install.skill]` (`append`, `replace`, or `create-if-missing`;
   `replace` is the right choice for a standalone skill file).
5. **Test locally.** `make build && ./bin/mure integration install <name>`
   followed by `mure integration list` and `mure integration uninstall`.
6. Open a PR. CI validates every manifest under `harnesses/` decodes
   strictly (`internal/harnesses` test suite).

The full schema lives in [PRD 005 §7](./specs/005-generalize-harness/PRD.md).

## Development

```sh
make build         # build ./bin/mure
make test          # go test ./... + shellcheck
make tmux-test     # real-tmux hook integration test
make acceptance    # run test/acceptance.sh end-to-end
make verify        # everything, including lint
```

Design notes and spec history live under [`specs/`](./specs/); the current
shape of the codebase is in [`specs/current.md`](./specs/current.md).

## License

TBD.
