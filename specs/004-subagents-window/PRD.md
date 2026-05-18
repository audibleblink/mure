# PRD 004 — `subagents-window` Spawn Target

## 1. Summary

Add a new value, `subagents-window`, to the `@mure-spawn-target` tmux
option. When selected, `mure spawn` collects all spawned subagent panes
into a single tmux window named `subagents` (one per session). The
window is created lazily on the first spawn and reused by subsequent
spawns — including those issued by worker agents.

This PRD also extracts the inline target switch in `cmdSpawn` into a
testable helper, `pickSpawnTarget`, behind a small `tmuxRunner` seam.

## 2. Goals

- Give users a one-line opt-in (`set -g @mure-spawn-target subagents-window`)
  to collect agent panes in a dedicated window instead of fragmenting
  the current window.
- Make the spawn-target decision unit-testable without invoking tmux.
- Zero changes to the daemon, sock protocol, roster, sidebar, focus/
  wait/ls, or the tmux plugin runtime (option-value docs only).

## 3. Non-Goals

- No pane cap, no axis logic, no auto-layout, no per-pane tagging tied
  to the window.
- No `mure subagents` subcommand, no `mure focus` changes, no sidebar
  awareness of the window.
- No migration of existing panes into a `subagents` window.
- No changes to other target modes (`right-of-active`, `below-active`,
  `new-window`).

## 4. Tech Stack

| Layer | Choice |
|---|---|
| Language | Go (existing `cmd/mure` package) |
| tmux invocation | Existing `tmuxCmd(ctx, args...)` helper in `cmd/mure/socket.go` |
| Test runner | `go test ./...` (existing) |
| Test seam | `type tmuxRunner func(args ...string) (string, error)` (idiomatic func type, mirrors how tests already mock tmux behavior) |
| Docs | Markdown tables in `README.md`, `tmux-mure/README.md`, `specs/001-init/PRD.md` |

No new dependencies.

## 5. Behavior

### 5.1 Target values

`@mure-spawn-target` accepted values, in precedence-of-documentation
order:

| Value | Behavior |
|---|---|
| `subagents-window` | **(new, default)** Spawn into the per-session `subagents` window; create it lazily. |
| `right-of-active` | Existing: `split-window -h` from the active pane. |
| `below-active` | Existing: `split-window -v` from the active pane. |
| `new-window` | Existing: `new-window` in the current session. |

### 5.2 Default change

The default value of `@mure-spawn-target` changes from `right-of-active`
to `subagents-window`. This affects:

- The fallback when the option is unset (e.g. plugin not installed).
- The documented default in all three docs files.

### 5.3 Unknown-value handling

If `@mure-spawn-target` is set to an unrecognized string, `mure spawn`
writes a single line to stderr:

```
mure spawn: unknown @mure-spawn-target %q; falling back to subagents-window
```

…and proceeds as `subagents-window`. Today's silent `default:` fallback
to `right-of-active` is removed.

### 5.4 `subagents-window` spawn algorithm

Scope: the **spawning pane's session**. Resolved via
`tmux display-message -p -t "$TMUX_PANE" '#{session_id}'` when
`TMUX_PANE` is set; otherwise via `display-message -p '#{session_id}'`
(current client's session, matching tmux's own resolution rules).

1. **Locate the subagents window** in that session:
   - Run: `list-windows -t <session_id> -F '#{window_id} #{window_name} #{@mure-subagents-window}'`.
   - Prefer the first row whose third column is `1` (carries the
     `@mure-subagents-window` marker option).
   - If none, fall back to the first row whose `window_name` is exactly
     `subagents`.
   - Otherwise, no match.

2. **No match** → create:
   - `new-window -d -t <session_id> -n subagents -P -F '#{pane_id}' <payload>`
   - Then tag the new window so subsequent spawns find it
     unambiguously: resolve the new pane's window id with
     `display-message -p -t <pane_id> '#{window_id}'` and run
     `set-option -w -t <window_id> @mure-subagents-window 1`.
   - The `-d` flag guarantees the user is **not** switched to the new
     window.

3. **Match found** → split:
   - `split-window -t <window_id> -P -F '#{pane_id}' <payload>`
   - tmux picks the split direction and the pane within the window
     (typically the active pane). No `select-window` is issued; the
     user's current view does not change.

4. **Per-pane bookkeeping** (`@mure-role`, `@mure-spawned-at`) is set
   on the returned `pane_id` exactly as today.

### 5.5 Other modes — unchanged

`right-of-active`, `below-active`, and `new-window` produce the same
tmux invocations as before. No new option lookups, no new window/pane
tagging for these modes.

### 5.6 Worker spawns

Worker agents (pi processes inside spawned panes) call `mure spawn` via
the existing `pi-mure` extension. They inherit `MURE_SOCKET` and run
the same binary; no special-casing is needed for nested spawns to land
in the same `subagents` window. Their `TMUX_PANE` resolves to the
worker's own pane, which is itself inside the subagents window — so the
session lookup in §5.4 yields the same session, and the marker lookup
finds the same window.

### 5.7 Untouched env / pane state

- `MURE_ENV=1`, `MURE_AGENT_ID`, `MURE_SOCKET`, `MURE_ROLE`,
  `MURE_TASK`, `MURE_AGENT_CMD` are set on the new pane exactly as
  today.
- Per-pane `@mure-role` and `@mure-spawned-at` are set exactly as
  today.

## 6. Code Changes

### 6.1 `cmd/mure/spawn_target.go` (new)

Exports:

```go
type tmuxRunner func(args ...string) (string, error)

// spawnTargetPlan describes the tmux invocation that should be used
// to create the new agent pane. The caller passes argv to tmuxCmd.
type spawnTargetPlan struct {
    Argv []string // args to pass to `tmux ...` to create the pane
    // PostCreate, if non-nil, runs after the pane id is known and may
    // issue follow-up tmux commands (e.g. tag the new window with
    // @mure-subagents-window=1). It receives the returned pane_id.
    PostCreate func(paneID string) error
}

// pickSpawnTarget resolves @mure-spawn-target into a concrete plan.
// `payload` is the shell command string to exec in the new pane.
// `stderr` receives warnings (e.g. unknown target values).
func pickSpawnTarget(
    run tmuxRunner,
    target string,
    payload string,
    stderr io.Writer,
) (spawnTargetPlan, error)
```

Internals:

- `subagents-window` is the default branch when `target` is empty or
  unrecognized (with the unknown-value stderr warning per §5.3).
- Session lookup helper:
  `resolveSessionID(run tmuxRunner) (string, error)` — uses
  `TMUX_PANE` if set, else `display-message -p '#{session_id}'`.
- Window lookup helper:
  `findSubagentsWindow(run, sessionID) (windowID string, found bool, err error)`
  — runs `list-windows -t <sessionID> -F '#{window_id} #{window_name} #{@mure-subagents-window}'`
  and applies the precedence in §5.4 step 1.

### 6.2 `cmd/mure/spawn.go` (edit)

Replace the inline `switch target { ... }` block (lines 36–46 today)
with:

```go
plan, err := pickSpawnTarget(tmuxRunnerFromCtx(ctx), target, payload, stderr)
if err != nil {
    fmt.Fprintf(stderr, "mure spawn: %v\n", err)
    return 1
}
paneID, err := tmuxCmd(ctx, plan.Argv...)
if err != nil {
    fmt.Fprintf(stderr, "mure spawn: %v\n", err)
    return 1
}
if plan.PostCreate != nil {
    if err := plan.PostCreate(paneID); err != nil {
        fmt.Fprintf(stderr, "mure spawn: %v\n", err)
        return 1
    }
}
```

Where `tmuxRunnerFromCtx(ctx)` is a one-line adapter:

```go
func tmuxRunnerFromCtx(ctx context.Context) tmuxRunner {
    return func(args ...string) (string, error) { return tmuxCmd(ctx, args...) }
}
```

Also remove the `if err != nil || target == "" { target = "right-of-active" }`
default. The default now lives inside `pickSpawnTarget` and is
`subagents-window`. The `show-option` lookup is left in place; an
error/empty result yields `target == ""`, which `pickSpawnTarget`
treats as "use default".

### 6.3 `cmd/mure/spawn_target_test.go` (new)

Pure unit tests against `pickSpawnTarget` with a recording fake
`tmuxRunner`. **No real tmux required.**

Table-driven over target values and window-presence combinations.

#### Fake runner shape

```go
type recRunner struct {
    responses map[string]string         // args-key -> stdout
    errs      map[string]error          // args-key -> err
    calls     [][]string                // recorded argv
}
```

Args-key is `strings.Join(args, " ")` (good enough; tests use unique
arg combinations).

#### Required cases

1. **`subagents-window` — window missing**
   - `show-option` returns `subagents-window`.
   - Session lookup returns `$1`.
   - `list-windows -t $1 ...` returns empty stdout.
   - Plan.Argv equals `["new-window", "-d", "-t", "$1", "-n", "subagents", "-P", "-F", "#{pane_id}", "<payload>"]`.
   - Invoking `plan.PostCreate("%42")`:
     - Issues `display-message -p -t %42 '#{window_id}'` (returns `@7`).
     - Then `set-option -w -t @7 @mure-subagents-window 1`.
   - No stderr output.

2. **`subagents-window` — window present via marker**
   - `list-windows` returns:
     ```
     @3 misc 
     @7 subagents 1
     @9 other 
     ```
   - Plan.Argv equals `["split-window", "-t", "@7", "-P", "-F", "#{pane_id}", "<payload>"]`.
   - Plan.PostCreate is `nil`.

3. **`subagents-window` — window present via name fallback only**
   - `list-windows` returns `@5 subagents ` (no marker column value).
   - Plan.Argv targets `@5`.

4. **`subagents-window` — marker beats name**
   - `list-windows` returns:
     ```
     @4 subagents 
     @8 agents 1
     ```
   - Plan.Argv targets `@8` (marker wins even when names differ).

5. **`right-of-active` — unchanged**
   - Plan.Argv equals `["split-window", "-h", "-P", "-F", "#{pane_id}", "<payload>"]`.
   - No session lookup, no `list-windows` call (verified via
     `recRunner.calls`).

6. **`below-active` — unchanged**
   - Plan.Argv equals `["split-window", "-v", "-P", "-F", "#{pane_id}", "<payload>"]`.

7. **`new-window` — unchanged**
   - Plan.Argv equals `["new-window", "-P", "-F", "#{pane_id}", "<payload>"]`.

8. **Empty target → default**
   - `target = ""` → behaves identically to case 1 (missing) or 2
     (present) given the same `list-windows` fixture.
   - No stderr warning.

9. **Unknown target → warn + default**
   - `target = "diagonal"` →
     - stderr contains `unknown @mure-spawn-target "diagonal"; falling back to subagents-window`.
     - Plan matches the `subagents-window` behavior for the fixture.

10. **Session lookup uses `TMUX_PANE` when set**
    - Test sets `TMUX_PANE=%99` (via `t.Setenv`); recorded call is
      `display-message -p -t %99 #{session_id}`.

11. **Session lookup falls back when `TMUX_PANE` unset**
    - `t.Setenv("TMUX_PANE", "")`; recorded call is
      `display-message -p #{session_id}`.

## 7. Docs Changes

### 7.1 `README.md`

In the tmux plugin options table:

```
| `@mure-spawn-target` | `subagents-window` |
```

(Was `right-of-active`.)

### 7.2 `tmux-mure/README.md`

```
| `@mure-spawn-target` | `subagents-window` | Read by `mure spawn`: `subagents-window`, `right-of-active`, `below-active`, `new-window`. Unknown values warn and fall back to `subagents-window`. |
```

### 7.3 `specs/001-init/PRD.md`

Two edits:

- Table row at line 249 — update default and value list to include
  `subagents-window` (new default).
- Text at line 387 — update the default-when-unset sentence to say
  `subagents-window`.

The `mure spawn` row at line 204 (describes target read) needs its
default updated to `subagents-window` as well.

## 8. Files Touched

- `cmd/mure/spawn.go` — edit (replace inline switch).
- `cmd/mure/spawn_target.go` — **new** (helper + runner type).
- `cmd/mure/spawn_target_test.go` — **new** (11 unit cases).
- `README.md` — one table cell.
- `tmux-mure/README.md` — one table row.
- `specs/001-init/PRD.md` — three lines.

**Not touched:** daemon, roster, sidebar, sock protocol, focus, wait,
ls, hook, doctor, up, down, tmux plugin runtime scripts, `pi-mure`
extension.

## 9. Acceptance Criteria

1. `go test ./cmd/mure/...` passes, including all 11 cases in §6.3.
2. All pre-existing tests pass unchanged.
3. With `@mure-spawn-target` unset (or set to `subagents-window`),
   running `mure spawn <role>` twice in a fresh tmux session:
   - First call creates a window named `subagents` (verifiable via
     `tmux list-windows`) and the user is **not** switched to it.
   - Second call adds a pane to that same window; window count is
     still 2 (original + subagents).
4. With `@mure-spawn-target right-of-active`, behavior is bit-identical
   to today (verified by case 5 + manual smoke).
5. With `@mure-spawn-target diagonal` (unknown), stderr emits the
   warning line and the spawn lands in the `subagents` window.
6. A worker agent that runs `mure spawn` inside its own pane (which
   lives in the `subagents` window) lands its child in the same
   window.
7. Docs in `README.md`, `tmux-mure/README.md`, and `specs/001-init/PRD.md`
   reflect the new default and the `subagents-window` value.

## 10. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Two `subagents` windows exist (user-created collision) | Marker option `@mure-subagents-window=1` is preferred over name match; the window mure creates always carries the marker. |
| User renames or closes the subagents window | Next spawn re-creates it (marker lookup fails → name lookup fails → create branch). Closing is a no-op in mure state. |
| `list-windows` format string parsing breaks on weird window names | Names with spaces are uncommon; parser splits each line into at most 3 whitespace-delimited fields with the third being optional. If parsing fails, treat as "not found" and create. |
| Default change surprises existing users | Called out explicitly in §5.2; users who want the old behavior set `@mure-spawn-target right-of-active` in their `.tmux.conf`. README + plugin README updated in the same PR. |
| Worker spawns race on first creation (two workers, no window yet) | Tolerated. Worst case: two `subagents` windows briefly exist; the marker mechanism means subsequent spawns deterministically pick the first marker-tagged one. Not worth locking for. |

## 11. Out-of-Scope Follow-Ups

- Per-spawn override flag (e.g. `mure spawn --target=new-window <role>`).
- `mure focus subagents` shortcut.
- Auto-layout inside the subagents window (tiled, main-vertical, etc.).
- Sidebar grouping by window.
- Migrating already-spawned panes into the subagents window.
