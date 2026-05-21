# tmux-mure

tmux plugin for [mure](https://github.com/<owner>/mure). Pure shell + tmux
config. Owns the tmux-side surfaces only:

- global hooks (`after-select-pane`, `pane-exited`, `session-closed`) that
  fork `mure _hook ...` so the daemon can observe focus/death/teardown,
- prefix-`M` toggle for the `mure sidebar` pane.

Agent status is intentionally **not** surfaced via tmux options or the
status line. Status is observable only through `mure ls` and the sidebar
(which reads from the daemon socket).

The plugin spawns no long-lived processes.

## Prerequisites

- tmux >= 3.2 (required for `set-hook -gu`).
- The `mure` binary must be on the `PATH` of whatever shell tmux's
  `run-shell` inherits (typically your login shell). All hooks and the
  sidebar toggle short-circuit with a message if `mure` is not found.

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

In all cases, first remove the hooks and key bind from the running tmux
server, then reload your config.

### TPM install

1. Remove the `set -g @plugin '<owner>/tmux-mure'` line from your
   `~/.tmux.conf`.
2. Run the uninstall script to clear hooks from the running server:
   ```sh
   ~/.tmux/plugins/tmux-mure/scripts/uninstall-hooks.sh
   ```
3. In tmux, press `prefix + alt-u` (TPM's clean) to remove the plugin
   directory.
4. Reload your config:
   ```sh
   tmux source-file ~/.tmux.conf
   ```

### Local-clone install

1. Remove the `run-shell /path/to/tmux-mure/tmux-mure.tmux` line from your
   `~/.tmux.conf`.
2. Run the uninstall script from your checkout:
   ```sh
   /path/to/mure/tmux-mure/scripts/uninstall-hooks.sh
   ```
3. Reload your config:
   ```sh
   tmux source-file ~/.tmux.conf
   ```
4. Delete the checkout if no longer needed.

## Options

| Option | Default | Meaning |
|---|---|---|
| `@mure-sidebar-width` | `36` | Sidebar pane width (columns / rows). |
| `@mure-sidebar-position` | `left` | `left`, `right`, `top`, `bottom`. |
| `@mure-sidebar-key` | `M` | Prefix-key for sidebar toggle. |
| `@mure-spawn-target` | `subagents-window` | Read by `mure spawn`. Either the reserved keyword `subagents-window` (find-or-create a dedicated window) or any tmux pane-creating command (e.g. `split-window -h`, `new-window -t :9`). The plugin rewrites legacy keywords `right-of-active`, `below-active`, `new-window` to their command equivalents at load time. mure appends `-P -F '#{pane_id}' <payload>` and runs it. |
| `@mure-plugin-version` | `1` | Set by the plugin; checked by `mure doctor`. |

Set any of these before TPM's `run` line.
