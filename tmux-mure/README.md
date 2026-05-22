# tmux-mure

tmux plugin for [mure](https://github.com/<owner>/mure). Pure shell + tmux
config. Owns the tmux-side surfaces only:

- prefix-`M` toggle for the `mure sidebar` pane,
- a default `@mure-spawn-target` (read by `mure spawn`).

The plugin installs no tmux hooks: pane death is observed by the daemon
directly via tmux control mode.

Agent status is intentionally **not** surfaced via tmux options or the
status line. Status is observable only through `mure ls` and the sidebar
(which reads from the daemon socket).

The plugin spawns no long-lived processes.

## Prerequisites

- tmux >= 3.2.
- The `mure` binary must be on the `PATH` of whatever shell tmux's
  `run-shell` inherits (typically your login shell). The sidebar toggle
  short-circuits with a message if `mure` is not found.

## Install from a local directory

If you already have this repo checked out, source the plugin file directly from
`.tmux.conf`:

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

## Install with TPM

```tmux
set -g @plugin '<owner>/tmux-mure'
run '~/.tmux/plugins/tpm/tpm'
```

Then `prefix + I` to fetch. See [`example.tmux.conf`](./example.tmux.conf) for
example overrides.

## Uninstall

### TPM install

1. Remove the `set -g @plugin '<owner>/tmux-mure'` line from your `~/.tmux.conf`.
2. `prefix + alt-u` (TPM's clean) to remove the plugin directory.
3. `tmux source-file ~/.tmux.conf`.

### Local-clone install

1. Remove the `run-shell /path/to/tmux-mure/tmux-mure.tmux` line.
2. `tmux source-file ~/.tmux.conf`. Delete the checkout if no longer needed.


## Options

| Option | Default | Meaning |
|---|---|---|
| `@mure-sidebar-width` | `36` | Sidebar pane width (columns / rows). |
| `@mure-sidebar-position` | `left` | `left`, `right`, `top`, `bottom`. |
| `@mure-sidebar-key` | `M` | Prefix-key for sidebar toggle. |
| `@mure-spawn-target` | `subagents-window` | Read by `mure spawn`. Either the reserved keyword `subagents-window` (find-or-create a dedicated window) or any tmux pane-creating command (e.g. `split-window -h`, `new-window -t :9`). The plugin rewrites legacy keywords `right-of-active`, `below-active`, `new-window` to their command equivalents at load time. mure appends `-P -F '#{pane_id}' <payload>` and runs it. |
| `@mure-plugin-version` | `1` | Set by the plugin; checked by `mure doctor`. |

Set any of these before TPM's `run` line.
