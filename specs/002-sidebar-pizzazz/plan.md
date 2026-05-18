# Execution Plan — PRD 002 Sidebar Pizzazz

Spec: `specs/002-sidebar-pizzazz/PRD.md`
Scope: `internal/sidebar/` only.

Verification baseline (used by every phase):
```sh
go build ./...
go test ./internal/sidebar/...
go vet ./...
```
All three must pass before a phase is considered complete. Phases additionally
add their own focused tests as the autonomous feedback signal.

## Chunk 1: All phases

## Phase 1: Theme + Brand foundation

**Depends on:** none

Introduce the palette, logo, and gradient helper as standalone, pure
units. No `model.go` behavior changes yet — existing rendering must keep
working with its current colors/strings so the existing test suite stays
green throughout this phase.

### Tasks

- [x] Create `internal/sidebar/theme.go`:
  - [x] `type Palette struct` with `lipgloss.AdaptiveColor` fields for
        every role in PRD §6.1: `AccentA`, `AccentB`, `Working`,
        `Blocked`, `Errored`, `Idle`, `Disconnected`, `Dim`,
        `SelectionBG`, `SelectionFG`.
  - [x] `var DefaultPalette = Palette{...}` populated with the exact
        Latte/Mocha hex values in PRD §6.1.
  - [x] `var active = DefaultPalette` (package-private) and
        `func ActivePalette() Palette { return active }`. No setter
        exported this iteration (PRD §8 — indirection only).
  - [x] `func gradientRGB(a, b lipgloss.AdaptiveColor, t float64, dark bool) lipgloss.Color`
        — linear blend on RGB of the active light/dark variant chosen
        by `dark`. Helper to parse `#rrggbb` → `(r,g,b)` lives in this
        file, unexported.
  - [x] `func RenderLogo(lines []string, p Palette, dark bool) []string`
        — per-rune linear gradient between `p.AccentA` and `p.AccentB`,
        `t = runeIndex / (totalRunes-1)` across the joined visible runes
        of all lines (so the gradient spans the whole logo, not each
        line independently). Spaces pass through uncolored.
- [x] Create `internal/sidebar/brand.go`:
  - [x] `var Logo = []string{...}` containing exactly the 3 ASCII-art
        lines from PRD §5.1.
  - [x] `const Wordmark = "mure"` (used by the narrow-width fallback in
        Phase 2).
  - [x] Doc comment on `Logo` declaring it the stable extension point
        (PRD §8).
- [x] Create `internal/sidebar/theme_test.go`:
  - [x] `TestPaletteHasAllRoles` — sanity: each field is non-zero
        (both Light and Dark set).
  - [x] `TestGradientRGB_Endpoints` — `t=0` returns AccentA variant,
        `t=1` returns AccentB variant (compare hex round-trip).
  - [x] `TestGradientRGB_Midpoint` — `t=0.5` returns the channel-wise
        average of the two endpoints (±1 per channel for rounding).
  - [x] `TestRenderLogo_PreservesLineCount` — output has
        `len(Logo)` lines and each line's *visible* rune count equals
        the corresponding input line's rune count (strip ANSI before
        counting; use a small `stripANSI` test helper in
        `internal/sidebar/testhelpers_test.go`).
  - [x] `TestRenderLogo_SpacesUncolored` — every space rune in the
        input appears as a literal space (no surrounding SGR) in the
        output.

### Verification (autonomous loop)

- [x] `go test ./internal/sidebar/...` is green.
- [x] `go vet ./...` is clean.
- [x] Existing `model_test.go` still passes unmodified (proves Phase 1
      is additive).

## Phase 2: Responsive layout, header, footer, dividers, selection

**Depends on:** Phase 1

Rewire `model.View` to produce the layout in PRD §5, including the
logo/wordmark switch, count line, dividers, footer, selection
highlight, and height-driven region dropping. No spinner animation and
no disconnected styling yet — those land in Phase 3 and Phase 4.

### Tasks

- [x] In `internal/sidebar/model.go`, extend `Model`:
  - [x] Fields: `width int`, `height int`, `palette Palette`,
        `dark bool` (default `true`; flipped by an internal helper —
        adaptive color handles real terminals, but `dark` controls the
        variant chosen for `RenderLogo`).
  - [x] `NewModel` initializes `width=36`, `height=0` (meaning "no
        height constraint applied"), `palette = ActivePalette()`,
        `dark = true`.
- [x] Handle `tea.WindowSizeMsg` in `Update`: store `Width`/`Height`,
      return `nil` cmd.
- [x] Replace `View` with a builder that composes regions in PRD §7.2
      priority order:
  - [x] Logo region: if `width-2 >= 27` render `RenderLogo(Logo, ...)`
        followed by one blank line (4 lines). Else render `Wordmark`
        styled in `AccentA` on a single line.
  - [x] Count line: `fmt.Sprintf("%d agents", len(agents))` in `Dim`.
  - [x] Top divider: a line of `─` of length `width-2` in `Dim`.
  - [x] Agent rows: existing column layout
        `" %s %-10s %-8s %s"` (selection marker, glyph, name, status,
        elapsed). Glyph + status label colored by status role from the
        palette. Elapsed colored `Dim` by default, `AccentA` once age
        > 5 min. Selected row: foreground = `SelectionFG`, full row
        padded to `width-2` and background-filled with `SelectionBG`;
        `▸` marker preserved.
  - [x] Bottom divider: same as top divider.
  - [x] Footer: `"j/k ↑↓  ⏎ focus  q quit"` in `Dim`.
  - [x] Every region truncated (rune-aware) to `width-2`.
- [x] Implement height-driven drop policy (PRD §7.2). Extract a small
      helper `layout(width, height int, regions []region) []string`
      inside `model.go` where `region` is a struct with `lines []string`
      and a `priority` enum. Drop order on insufficient height:
      `footer → footerDivider → logo → wordmark → countLine`. Agent
      rows are clipped from the bottom only after all of the above
      have been dropped; clip window is shifted so the selected row
      stays visible.
  - [x] Helper lives in `model.go`. If it grows past ~120 lines, split
        into `internal/sidebar/layout.go` at that point (judgement call
        deferred until written).
- [x] Wrap the whole rendered block in the existing rounded `lipgloss`
      border, border color = `AccentA` (connected state for now).
- [x] Rewrite `internal/sidebar/model_test.go`:
  - [x] Add a `stripANSI` helper (shared with Phase 1 tests).
  - [x] Golden tests assert **layout + raw text** (ANSI-stripped) per
        PRD §12. Goldens live in `internal/sidebar/testdata/`.
  - [x] `TestView_DefaultWidth` — width 36, 3 agents (one of each of
        idle/working/blocked), no selection: matches a new golden.
  - [x] `TestView_NarrowWidthDropsLogo` — `WindowSizeMsg{Width:20}`:
        no rune extends past column 18; first content line equals
        `mure` (after border + leading padding).
  - [x] `TestView_SmallHeightDropsRegions` — feed decreasing heights
        and assert the drop order from PRD §7.2 (footer goes first,
        etc.).
  - [x] `TestView_SelectedRowStaysVisible` — many agents, small
        height, selected row past the visible window; assert the
        selected agent name appears in the rendered output.
  - [x] `TestView_SelectionStyling` — selected row contains the `▸`
        marker and the row text occupies exactly `width-2` columns
        (after ANSI strip).
  - [x] `TestKeybindingsUnchanged` — `j/k`, `↑/↓`, `enter`, `q` move
        selection / quit exactly as today (mirror current behavior;
        PRD §10.8).

### Verification (autonomous loop)

- [x] `go test ./internal/sidebar/...` green.
- [x] `go vet ./...` clean.
- [x] Manual smoke (optional, non-blocking): `go run ./cmd/mure sidebar`
      against a stub daemon renders without panics. Not part of the
      automated loop.

## Phase 3: Gated spinner

**Depends on:** Phase 2

Add the working-glyph animation with strict gating: zero ticks
scheduled when no agent is `working`.

### Tasks

- [x] In `model.go`:
  - [x] Add `tick uint64` to `Model`.
  - [x] Define unexported `type tickMsg struct{}` and
        `spinnerFrames = []rune{'⠋','⠙','⠹','⠸','⠼','⠴','⠦','⠧','⠇','⠏'}`.
  - [x] `func anyWorking(agents []Agent) bool` helper.
  - [x] `func scheduleTick() tea.Cmd { return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} }) }`.
  - [x] In `Update`:
    - [x] On `tickMsg`: increment `m.tick`; return `scheduleTick()`
          iff `anyWorking(m.agents)`, else `nil`.
    - [x] On the existing frame-applied path (the daemon-state update
          handler — currently `frameMsg` in `model.go`): if applying
          the frame causes `anyWorking` to transition false→true,
          return `scheduleTick()` batched with any existing cmd.
  - [x] In `Init`: return a batch of the existing `waitFrame` cmd plus
        a conditional `scheduleTick()` only if the initial agent set
        already has a working agent (in practice empty at startup, so
        usually just `waitFrame`).
  - [x] In the agent-row renderer, the `working` glyph is
        `spinnerFrames[m.tick % uint64(len(spinnerFrames))]`. All other
        glyphs remain static.
- [x] Tests in `model_test.go`:
  - [x] `TestSpinner_NoTicksWhenNoWorking` — construct a model with
        agents in every non-working status; call `Init()` and feed an
        update that doesn't introduce a working agent; assert the
        returned `tea.Cmd` chain produces **no `tickMsg`**. (Run
        returned cmds via a small harness that records emitted msgs
        without sleeping; reject `tickMsg`. PRD §10.5.)
  - [x] `TestSpinner_TickArmedOnTransition` — start with no working
        agents (no tick scheduled); apply a frame that introduces a
        working agent; assert a tick cmd is returned.
  - [x] `TestSpinner_FrameAdvances` — start with one working agent;
        feed two `tickMsg`s; assert the rendered working glyph differs
        between renders and matches `spinnerFrames[0]` then
        `spinnerFrames[1]`.
  - [x] `TestSpinner_StopsWhenWorkAtComplete` — start working, apply
        a frame that moves the agent to `idle`; next `tickMsg` returns
        `nil` (no re-arm).

### Verification (autonomous loop)

- [x] `go test ./internal/sidebar/...` green, including all four new
      spinner tests.
- [x] `go vet ./...` clean.

## Phase 4: Disconnected state + final acceptance

**Depends on:** Phase 3

Wire the disconnected visuals and close out every remaining acceptance
criterion in PRD §10.

### Tasks

- [x] In `model.go`:
  - [x] Confirm `Model` already tracks connection state (it does:
        existing `connected bool` or equivalent — wire to whatever the
        current field is; if absent, add `connected bool` updated on
        the existing daemon-disconnect path).
  - [x] Border color: `AccentA` when connected, `Errored` when not.
  - [x] When `!connected`, insert a `(disconnected)` line in `Errored`
        immediately above the **top** divider. Logo and other regions
        are **not** dimmed (PRD §5.6).
  - [x] The `(disconnected)` line participates in the drop policy at
        the same priority as the count line (dropped before agent
        rows, after the footer/dividers/logo).
- [x] Tests in `model_test.go`:
  - [x] `TestView_DisconnectedBorderColor` — after a disconnect event,
        the rendered output's border SGR sequence matches the
        `Errored` color (raw, not ANSI-stripped). Use a tiny helper
        that extracts the first SGR sequence on the top border line.
  - [x] `TestView_DisconnectedLinePresent` — `(disconnected)` appears
        above the top divider; logo line characters remain
        byte-identical to the connected golden (proves "not dimmed").
  - [x] `TestAdaptiveColorsRespected` — render once with `dark=true`
        and once with `dark=false`; assert at least one glyph's SGR
        sequence differs between the two (proves the adaptive variant
        is actually consulted). PRD §10.2.
- [x] Cross-package safety check:
  - [x] `go test ./...` — every package outside `internal/sidebar/`
        passes unchanged. PRD §10.9.
- [x] Walk PRD §10 acceptance criteria 1–10 and tick each off against
      a specific test name:
  - [x] §10.1 default render → `TestView_DefaultWidth`
  - [x] §10.2 adaptive colors → `TestAdaptiveColorsRespected`
  - [x] §10.3 narrow width → `TestView_NarrowWidthDropsLogo`
  - [x] §10.4 small height → `TestView_SmallHeightDropsRegions` +
        `TestView_SelectedRowStaysVisible`
  - [x] §10.5 zero ticks when idle → `TestSpinner_NoTicksWhenNoWorking`
  - [x] §10.6 spinner advances → `TestSpinner_FrameAdvances`
  - [x] §10.7 disconnect visuals → `TestView_DisconnectedBorderColor`
        + `TestView_DisconnectedLinePresent`
  - [x] §10.8 keybindings → `TestKeybindingsUnchanged`
  - [x] §10.9 other packages unchanged → `go test ./...`
  - [x] §10.10 goldens updated → covered by Phase 2 + this phase

### Verification (autonomous loop)

- [x] `go test ./...` green across the whole repo.
- [x] `go vet ./...` clean.
- [x] Grep check: `git diff --stat` touches only files listed in
      PRD §13 (`internal/sidebar/model.go`,
      `internal/sidebar/brand.go`, `internal/sidebar/theme.go`,
      `internal/sidebar/model_test.go`, plus new
      `internal/sidebar/theme_test.go` and
      `internal/sidebar/testhelpers_test.go`, plus
      `internal/sidebar/testdata/*`). Anything else → fail and revert.
