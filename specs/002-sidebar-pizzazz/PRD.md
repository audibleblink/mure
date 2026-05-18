# PRD 002 — Sidebar Pizzazz

## 1. Summary

Visual and behavioral refresh of the Bubble Tea sidebar (`mure sidebar`).
Adds a branded gradient header, an adaptive Catppuccin-based color palette,
responsive sizing, a footer with keybinding hints, dividers, and a gated
spinner for working agents. Scope is limited to `internal/sidebar/`; no
changes to the daemon, sock protocol, CLI, or tmux plugin.

## 2. Goals

- Make the sidebar visually distinctive and informative at a glance.
- Adapt cleanly to the tmux pane's actual width and height.
- Keep the visual language easily themeable later (palette + logo as
  package-level vars; future tmux-option theming out of scope).

## 3. Non-Goals

- No scrolling/viewport when agents exceed available height.
- No componentization into multiple files beyond `model.go` + a small
  `brand.go`/`theme.go` (Option B in research is deferred).
- No reading from `@mure-color-*` or `@mure-sidebar-*` tmux options at
  runtime (Option C deferred).
- No changes to keybindings, socket frames, or agent state machine.
- No animated transitions beyond the spinner.

## 4. Tech Stack

Confirmed unchanged from PRD 001:

| Layer | Choice |
|---|---|
| TUI framework | `github.com/charmbracelet/bubbletea` (already in `go.mod`) |
| Styling | `github.com/charmbracelet/lipgloss` (already in `go.mod`) |
| Color model | `lipgloss.AdaptiveColor` with Catppuccin Latte (light) /
  Mocha (dark) hex values |
| Spinner | Hand-rolled via `tea.Tick`; **no** `bubbles/spinner` dependency |
| Logo source | Package-level `var Logo []string` in `internal/sidebar/brand.go` |

No new direct dependencies.

## 5. User-Visible Layout

Top-to-bottom, rendered inside a single rounded `lipgloss` border:

```
┌──────────────────────────────────┐
│ ███▄███▄ ██ ██ ████▄ ▄█▀█▄       │  ← logo (gradient, 3 lines)
│ ██ ██ ██ ██ ██ ██ ▀▀ ██▄█▀       │
│ ██ ██ ██ ▀██▀█ ██    ▀█▄▄▄       │
│                                  │  ← 1 blank
│ 3 agents                         │  ← count line (dim)
│ ────────────────────────────     │  ← divider
│ ▸ ⠋ planner    working    0:14   │  ← agent rows
│   ◐ builder    blocked    1:02   │
│   ○ reviewer   idle       —      │
│ ────────────────────────────     │  ← divider
│ j/k ↑↓  ⏎ focus  q quit          │  ← footer (dim)
└──────────────────────────────────┘
```

### 5.1 Logo
- Default value (in `brand.go`):
  ```
  ███▄███▄ ██ ██ ████▄ ▄█▀█▄
  ██ ██ ██ ██ ██ ██ ▀▀ ██▄█▀
  ██ ██ ██ ▀██▀█ ██    ▀█▄▄▄
  ```
  Followed by one blank line of breathing room (4 lines total in the
  rendered region).
- Width ≈ 27 cols. Pane width defaults to 36 (PRD 001 §7).
- Rendered with a per-rune linear gradient between two adaptive accent
  colors (see §6).

### 5.2 Header count line
- Format: `"%d agents"` (no `mure ·` prefix; brand is now the logo).
- Dim style.

### 5.3 Agent rows
- Same column layout as today: `" %s %-10s %-8s %s"`
  (selection-marker, glyph, name, status label, elapsed).
- Glyph and status label colored per status (§6.2).
- Elapsed colored dim by default; brighter once age > 5 min.
- Selected row: foreground swapped to selection-fg; full-width background
  fill in selection-bg. `▸` marker kept.

### 5.4 Spinner (gated)
- Only the `working` glyph animates. All others remain static.
- Frames: `⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏` at 120 ms cadence.
- The tick command is **only scheduled when ≥1 agent has status
  `working`**. When no agent is working, the program is fully
  event-driven (no idle CPU).

### 5.5 Footer
- Single line: `j/k ↑↓  ⏎ focus  q quit`, dim style, preceded by a
  divider line.

### 5.6 Border
- Rounded border, accent color when connected, red when disconnected.
- When disconnected: extra line `(disconnected)` rendered above the
  divider, in red. Logo and other elements are **not** dimmed.

## 6. Theme

### 6.1 Palette (Catppuccin)
Adaptive — `lipgloss.AdaptiveColor{Light: <Latte>, Dark: <Mocha>}`.

| Role | Light (Latte) | Dark (Mocha) |
|---|---|---|
| Accent A (logo gradient start, border) | `#8839ef` mauve | `#cba6f7` mauve |
| Accent B (logo gradient end) | `#ea76cb` pink | `#f5c2e7` pink |
| Status working | `#40a02b` green | `#a6e3a1` green |
| Status blocked | `#df8e1d` yellow | `#f9e2af` yellow |
| Status errored | `#d20f39` red | `#f38ba8` red |
| Status idle | `#6c6f85` overlay1 | `#9399b2` overlay2 |
| Status disconnected | `#8c8fa1` overlay2 | `#7f849c` overlay1 |
| Dim (count, footer, elapsed) | `#6c6f85` | `#9399b2` |
| Selection bg | `#dce0e8` crust | `#313244` surface0 |
| Selection fg | `#4c4f69` text | `#cdd6f4` text |

All values defined in `internal/sidebar/theme.go` as a `Palette` struct
with adaptive-color fields. Easily swappable later.

### 6.2 Application
- Glyph + status-label colored by status role.
- Logo: split rune index across gradient (linear blend on RGB of the
  active light/dark variant).
- Border: accent A when connected, errored color when not.

## 7. Responsiveness

### 7.1 Width
- Listen for `tea.WindowSizeMsg`; store `width`.
- Default width before first resize: 36.
- All rendered lines are truncated to `width - 2` (border).
- **If `width - 2 < 27` (logo doesn't fit):** hide logo entirely; show
  plain text wordmark `mure` in accent A on a single line in its place.

### 7.2 Height
- Listen for `tea.WindowSizeMsg`; store `height`.
- Layout regions in priority order (kept top-down on drop):
  1. Logo block (4 lines) or wordmark fallback (1 line)
  2. Count line (1 line)
  3. Divider (1 line)
  4. Agent rows (variable; capped to remaining)
  5. Divider (1 line)
  6. Footer (1 line)
- When height is insufficient, **drop in this order** until it fits:
  footer → footer divider → logo (replaced by wordmark) → wordmark →
  count line. Agent rows are clipped from the **bottom** only after the
  above have been dropped. Selected agent is always kept visible by
  shifting the clip window if necessary.

## 8. Configurability (this iteration)

- `internal/sidebar/brand.go` exposes `var Logo []string`. Replacing the
  slice changes the rendered logo. Documented as the stable extension
  point for a future tmux option / config-file mechanism.
- `internal/sidebar/theme.go` exposes `var DefaultPalette Palette` and a
  `func ActivePalette() Palette` indirection so a future setter can swap
  it without touching call sites.
- **Not in scope:** env vars, tmux options, config files, runtime
  reload.

## 9. State & Update Model

No new state on the wire. Internal `Model` gains:
- `width, height int`
- `tick uint64` (spinner frame counter; advances only when a tick fires)
- `palette Palette` (resolved at `NewModel` time)

`Update` additions:
- `tea.WindowSizeMsg` → updates dimensions, returns `nil` cmd.
- `tickMsg` (internal) → increments `tick`; if `anyWorking(agents)` then
  re-arm `tea.Tick(120ms)`; otherwise no cmd.
- `frameMsg` handler: if applying the frame causes `anyWorking` to
  transition false→true, schedule a tick.

`Init` returns a batch of `waitFrame` and (conditionally) a tick.

## 10. Acceptance Criteria

1. Sidebar renders logo, count, agent list, and footer at default width
   36 with no overflow.
2. Status glyphs and labels render in the colors specified in §6.1 under
   both light and dark terminal backgrounds (adaptive).
3. Sending `tea.WindowSizeMsg{Width: 20}` replaces the logo with the
   `mure` wordmark; no rune extends past column 18.
4. Sending a small-height `WindowSizeMsg` drops regions in the order
   defined in §7.2; the selected agent row remains visible.
5. When all agents are `idle`/`blocked`/`errored`/`disconnected`, the
   program performs zero `tea.Tick` re-arms (verified in test by counting
   scheduled tick cmds).
6. When at least one agent is `working`, the working glyph advances
   through the spinner frames at ~120 ms cadence.
7. When the daemon disconnects, the border switches to the errored
   color and a `(disconnected)` line appears above the top divider. The
   logo remains undimmed.
8. Existing keybindings (`j/k`, `↑/↓`, `enter`, `q`) behave unchanged.
9. All existing daemon/sock/CLI tests still pass without modification.
10. `internal/sidebar/model_test.go` is updated with new golden output
    matching §5; new tests added for (3), (4), (5), (6), (7).

## 11. Out-of-Scope Follow-Ups (tracked here, not built)

- Tmux-option theming (`@mure-sidebar-logo`, `@mure-color-*`).
- Per-component file split (header/row/footer).
- Scrolling viewport.
- Animated transitions (focus changes, status changes).
- Configurable footer hints / keymap display.

## 12. Risk & Mitigation

| Risk | Mitigation |
|---|---|
| Golden-string tests become brittle to color codes | Strip ANSI in goldens; assert layout + raw text separately from styling |
| Spinner tick floods slow terminals | Gated to only when working agents exist; 120 ms cadence (~8 fps) |
| Catppuccin colors look wrong on 16-color terminals | Lipgloss degrades automatically; verify in CI on a `TERM=xterm` smoke test (informational, not blocking) |
| Logo Unicode width assumptions break in some fonts | Width fallback (§7.1) hides logo at narrow widths; documented |

## 13. Files Touched

- `internal/sidebar/model.go` (modified)
- `internal/sidebar/brand.go` (new — `var Logo []string`)
- `internal/sidebar/theme.go` (new — `Palette`, `DefaultPalette`,
  `ActivePalette`, gradient helper)
- `internal/sidebar/model_test.go` (rewritten goldens + new cases)

No changes to: `internal/sidebar/client.go`, `internal/sock/*`,
`internal/daemon/*`, `cmd/mure/*`, `tmux-mure/*`, `pi-mure/*`.
