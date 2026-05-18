# Execution Plan — PRD 004: `subagents-window` Spawn Target

Spec: `specs/004-subagents-window/PRD.md`
Implementation surface: `cmd/mure/spawn.go` (edit), `cmd/mure/spawn_target.go` (new), `cmd/mure/spawn_target_test.go` (new), three docs files. No other Go files touched. No new dependencies.

## Resolved contracts (locked before planning)

Confirmed by reading the current source:

- `tmuxCmd(ctx context.Context, args ...string) (string, error)` lives in `cmd/mure/socket.go:87`. It returns stdout trimmed of trailing whitespace (no terminal newline) and an error whose message includes stderr. The PRD's `tmuxRunner` type drops the `ctx` parameter — the production adapter (`tmuxRunnerFromCtx`) closes over `ctx`.
- Existing `cmdSpawn` (`cmd/mure/spawn.go`) reads `@mure-spawn-target` via `tmuxCmd(ctx, "show-option", "-gv", "@mure-spawn-target")` and falls back to `right-of-active` on error or empty. This fallback moves into `pickSpawnTarget` per PRD §6.2.
- Per-pane bookkeeping (`@mure-role`, `@mure-spawned-at`) and the spawn payload builder (`spawnPayload`) are not touched. They stay exactly where they are.
- The e2e harness in `test/e2e/e2e_test.go` already drives a real tmux server via `MURE_TMUX_SOCKET`, runs the real `mure` binary, and reads `list-panes` output. The existing test function is `TestEndToEnd` (the file has `//go:build e2e`, so the suite must be invoked with `-tags e2e`). We extend it (not replace it) for Phase 2 acceptance.
- `list-windows -F '#{window_id} #{window_name} #{@mure-subagents-window}'` emits one line per window. Unset user-options render as empty (tmux prints the literal empty string for the third column). Parsing splits on whitespace and accepts 2 or 3 tokens per line; a missing third token is treated as "no marker".
- `recRunner` in the unit tests uses `strings.Join(args, " ")` as the lookup key. All test fixtures use whitespace-free argv tokens (pane ids like `%42`, window ids like `@7`, session ids like `$1`), so this is unambiguous.

These decisions are final for this plan.

Phases are sized so each one leaves the repo compiling, all tests green, and a coherent slice of behavior shipped. Each phase ends with an autonomous verification command.

---

## Chunk 1: Plan

## Phase 1: Extract `pickSpawnTarget` helper (behavior-preserving refactor)
**Depends on:** none

Move the existing `switch target { ... }` block out of `cmdSpawn` and into a pure, testable helper in a new file. **No behavior change in this phase.** The default is still `right-of-active`, no unknown-value warning is emitted, and there is no `subagents-window` branch yet. This gives Phase 2 a clean seam to extend.

### Scope

#### New file: `cmd/mure/spawn_target.go`

Exactly these exports/types (PRD §6.1, minus the `subagents-window` logic which lands in Phase 2):

```go
package main

import (
    "fmt"
    "io"
)

// tmuxRunner is a seam over tmuxCmd that lets tests stub tmux invocations.
type tmuxRunner func(args ...string) (string, error)

// spawnTargetPlan describes the tmux invocation that should be used to create
// the new agent pane. Argv is passed to tmuxCmd by the caller. PostCreate, if
// non-nil, runs after the pane id is known and may issue follow-up tmux
// commands. It receives the returned pane_id.
type spawnTargetPlan struct {
    Argv       []string
    PostCreate func(paneID string) error
}

// pickSpawnTarget resolves @mure-spawn-target into a concrete plan.
// payload is the shell command string to exec in the new pane.
// stderr receives warnings (e.g. unknown target values).
func pickSpawnTarget(run tmuxRunner, target string, payload string, stderr io.Writer) (spawnTargetPlan, error) {
    switch target {
    case "new-window":
        return spawnTargetPlan{Argv: []string{"new-window", "-P", "-F", "#{pane_id}", payload}}, nil
    case "below-active":
        return spawnTargetPlan{Argv: []string{"split-window", "-v", "-P", "-F", "#{pane_id}", payload}}, nil
    case "right-of-active", "":
        return spawnTargetPlan{Argv: []string{"split-window", "-h", "-P", "-F", "#{pane_id}", payload}}, nil
    default:
        // Phase 1 preserves today's silent fallback to right-of-active.
        // Phase 2 replaces this with a warn-and-default-to-subagents-window branch.
        return spawnTargetPlan{Argv: []string{"split-window", "-h", "-P", "-F", "#{pane_id}", payload}}, nil
    }
}
```

`run` is accepted but unused in Phase 1; this keeps the signature stable across phases so Phase 2 only adds branches.

#### Edit: `cmd/mure/spawn.go`

Replace the inline switch (lines ~42–50) and the surrounding spawn call with the plan-driven path. Concretely:

- Remove the `switch target { ... } paneID, err := tmuxCmd(ctx, splitArgs...)` block.
- Replace with (note: the Phase 1 `pickSpawnTarget` never returns a non-nil error — the `if err != nil` block below is unreachable in Phase 1 and exists so Phase 2's additions, which *can* fail on tmux lookups, slot in without further edits to this site):
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
- Add the one-line adapter (kept in `spawn.go` since it closes over `ctx`):
  ```go
  func tmuxRunnerFromCtx(ctx context.Context) tmuxRunner {
      return func(args ...string) (string, error) { return tmuxCmd(ctx, args...) }
  }
  ```
- **Leave** the `if err != nil || target == "" { target = "right-of-active" }` default *for this phase only*. Phase 2 deletes it. This keeps Phase 1 behavior-identical.

#### New file: `cmd/mure/spawn_target_test.go`

Add the unchanged-mode unit tests only (PRD §6.3 cases 5, 6, 7). Subagents-window cases land in Phase 2.

Provide the shared fake-runner helper now so Phase 2 only adds rows:

```go
type recRunner struct {
    responses map[string]string
    errs      map[string]error
    calls     [][]string
}

func newRecRunner() *recRunner {
    return &recRunner{responses: map[string]string{}, errs: map[string]error{}}
}

func (r *recRunner) run(args ...string) (string, error) {
    r.calls = append(r.calls, append([]string(nil), args...))
    key := strings.Join(args, " ")
    if err, ok := r.errs[key]; ok {
        return "", err
    }
    return r.responses[key], nil
}
```

Tests (each one asserts `plan.Argv` via `reflect.DeepEqual`, that `plan.PostCreate == nil`, and that `r.calls` is empty — none of these modes should invoke tmux during planning):

- [x] `TestPickSpawnTarget_RightOfActive` — `target="right-of-active"`, payload `"PL"`; expect `Argv == []string{"split-window","-h","-P","-F","#{pane_id}","PL"}`; `len(r.calls) == 0`.
- [x] `TestPickSpawnTarget_BelowActive` — `target="below-active"`; expect `Argv == []string{"split-window","-v","-P","-F","#{pane_id}","PL"}`; `len(r.calls) == 0`.
- [x] `TestPickSpawnTarget_NewWindow` — `target="new-window"`; expect `Argv == []string{"new-window","-P","-F","#{pane_id}","PL"}`; `len(r.calls) == 0`.
- [x] `TestPickSpawnTarget_EmptyDefaultsToRightOfActive_Phase1` — `target=""`; same expectation as right-of-active. **Test name and assertion change in Phase 2** to default to subagents-window; this is intentional and called out so the Phase 2 diff is small.

Each test uses a `bytes.Buffer` for `stderr` and asserts it is empty (`buf.Len() == 0`).

### Implementation checklist

- [x] Create `cmd/mure/spawn_target.go` with `tmuxRunner`, `spawnTargetPlan`, and Phase 1 `pickSpawnTarget`.
- [x] Edit `cmd/mure/spawn.go`: add `tmuxRunnerFromCtx`, replace the inline switch with the plan-driven block, leave the existing `target=""` default in place.
- [x] Create `cmd/mure/spawn_target_test.go` with the four cases above and the `recRunner` helper.
- [x] Run `gofmt -w` on touched files.

### Autonomous verification

```bash
cd /Users/blink/Code/mure
gofmt -l cmd/mure/spawn.go cmd/mure/spawn_target.go cmd/mure/spawn_target_test.go
# Expected: empty output.
go build ./...
# Expected: clean build.
go test ./cmd/mure/... -count=1 2>&1 | tee /tmp/phase1.log
grep -E "^(FAIL|ok)" /tmp/phase1.log
# Expected: every package line begins with "ok"; no FAIL.
go test ./... -count=1 -run . -short 2>&1 | tail -20
# Expected: all packages pass (no behavior change).
```

Phase passes when: `gofmt` clean, `go build ./...` clean, all `cmd/mure` tests pass, full-repo `go test` passes (same set as before this phase).

---

## Phase 2: Add `subagents-window` mode + unknown-value warning + default change
**Depends on:** Phase 1

Implement the new branch end-to-end: subagents-window discovery (marker → name → create), `PostCreate` window tagging, unknown-value warning, and default change from `right-of-active` to `subagents-window`. Cover with the eight remaining unit cases plus one new e2e case.

### Scope

#### Edit: `cmd/mure/spawn_target.go`

Add (in this file, all unexported except the ones the PRD already lists as exported):

- `func resolveSessionID(run tmuxRunner) (string, error)` — checks `os.Getenv("TMUX_PANE")`:
  - non-empty → `run("display-message","-p","-t",pane,"#{session_id}")`,
  - empty → `run("display-message","-p","#{session_id}")`.
  - Returns the trimmed string (tmuxCmd already trims; the seam contract is identical).
- `func findSubagentsWindow(run tmuxRunner, sessionID string) (windowID string, found bool, err error)` — runs
  ```
  list-windows -t <sessionID> -F #{window_id} #{window_name} #{@mure-subagents-window}
  ```
  (each `-F` argument is the literal format string, **not** quoted — tmux receives the format via argv, not the shell). Then per line:
  - Split on whitespace; require 2 or 3 fields.
  - First pass: pick the first line whose third field is exactly `"1"`. Return its first field.
  - If no marker hit, second pass: pick the first line whose second field is exactly `"subagents"`. Return its first field.
  - Else `found=false`.
- Extend `pickSpawnTarget` switch:
  - Add a `"subagents-window"` case (and route empty `target` to it).
  - Unknown branch: write `mure spawn: unknown @mure-spawn-target %q; falling back to subagents-window\n` to `stderr` then fall through to the subagents-window branch.
  - Subagents-window logic:
    1. `sessionID, err := resolveSessionID(run)`; on error return `spawnTargetPlan{}, err`.
    2. `windowID, found, err := findSubagentsWindow(run, sessionID)`; on error return `spawnTargetPlan{}, err`.
    3. If `found`:
       ```go
       return spawnTargetPlan{Argv: []string{"split-window","-t",windowID,"-P","-F","#{pane_id}",payload}}, nil
       ```
       (No pane is specified inside the window — tmux splits the window's active pane by default. This matches PRD §5.4 step 3 and is intentional.)
    4. If not found:
       ```go
       return spawnTargetPlan{
           Argv: []string{"new-window","-d","-t",sessionID,"-n","subagents","-P","-F","#{pane_id}",payload},
           PostCreate: func(paneID string) error {
               wid, err := run("display-message","-p","-t",paneID,"#{window_id}")
               if err != nil { return err }
               _, err = run("set-option","-w","-t",wid,"@mure-subagents-window","1")
               return err
           },
       }, nil
       ```
  - Remove the old `default:` silent-right-of-active fallback.

Add `os` to the import set.

#### Edit: `cmd/mure/spawn.go`

- Delete the `if err != nil || target == "" { target = "right-of-active" }` line. Replace with:
  ```go
  if err != nil {
      target = ""
  }
  ```
  Empty target is now interpreted by `pickSpawnTarget` as "use default" (= subagents-window).

#### Edit: `cmd/mure/spawn_target_test.go`

Update the Phase 1 default case and add the subagents-window cases:

- [x] **Update** `TestPickSpawnTarget_EmptyDefaultsToRightOfActive_Phase1` → rename to `TestPickSpawnTarget_EmptyDefaultsToSubagentsWindow`. Assert it behaves identically to the missing-window case (#1 below) given the same fixture, and that `stderr` is empty.
- [x] **Test 1: `subagents-window` — window missing.** Fixture:
  - `t.Setenv("TMUX_PANE","")`.
  - `responses["display-message -p #{session_id}"] = "$1"`.
  - `responses["list-windows -t $1 -F #{window_id} #{window_name} #{@mure-subagents-window}"] = ""`.
  - Assert `plan.Argv == []string{"new-window","-d","-t","$1","-n","subagents","-P","-F","#{pane_id}","PL"}`.
  - Assert `plan.PostCreate != nil`. Then call `plan.PostCreate("%42")` with new fixture rows:
    - `responses["display-message -p -t %42 #{window_id}"] = "@7"`.
    - `responses["set-option -w -t @7 @mure-subagents-window 1"] = ""`.
  - Assert `PostCreate` returns `nil`, and that `r.calls` contains both follow-up argv vectors in order.
- [x] **Test 2: `subagents-window` — window present via marker.** `list-windows` stdout:
  ```
  @3 misc 
  @7 subagents 1
  @9 other 
  ```
  Assert `plan.Argv == []string{"split-window","-t","@7","-P","-F","#{pane_id}","PL"}`, `plan.PostCreate == nil`.
- [x] **Test 3: `subagents-window` — window present via name only.** `list-windows` stdout `"@5 subagents"`. Assert plan targets `@5`. (No marker column present.)
- [x] **Test 4: `subagents-window` — marker beats name.** stdout:
  ```
  @4 subagents 
  @8 agents 1
  ```
  Assert plan targets `@8`.
- [x] **Test 9: unknown target.** `target="diagonal"`, fixture identical to Test 2. Assert stderr matches `unknown @mure-spawn-target "diagonal"; falling back to subagents-window` (substring match). Assert `plan.Argv == []string{"split-window","-t","@7","-P","-F","#{pane_id}","PL"}`.
- [x] **Test 10: session lookup uses `TMUX_PANE` when set.** `t.Setenv("TMUX_PANE","%99")`. Fixture only needs `responses["display-message -p -t %99 #{session_id}"] = "$2"` and an empty `list-windows -t $2 ...` reply. Assert `r.calls[0] == []string{"display-message","-p","-t","%99","#{session_id}"}`.
- [x] **Test 11: session lookup fallback when `TMUX_PANE` unset.** `t.Setenv("TMUX_PANE","")`. Assert `r.calls[0] == []string{"display-message","-p","#{session_id}"}`.

Notes:
- Cases #5/#6/#7 from PRD §6.3 are already covered by Phase 1 tests; no duplication.
- Test #8 (empty target → default) is the renamed Phase 1 test above.

#### e2e test addition: `test/e2e/e2e_test.go`

The file is `//go:build e2e`-gated; add a sibling test function `TestEndToEndSubagentsWindow` (matches the existing `TestEndToEnd` naming). It must:

1. Set up the harness identically to `TestEndToEnd` (real tmux server on `MURE_TMUX_SOCKET`, plugin loaded). Do **not** set `@mure-spawn-target` — exercise the new default.
2. Capture the pre-spawn active window id (`tmux display-message -p '#{window_id}'`).
3. Run `mure spawn dummy` twice. Capture both `agent_id pane_id` outputs.
4. Assert via tmux that:
   - There are exactly 2 windows in the session (the original session window + `subagents`).
   - The `subagents` window's `@mure-subagents-window` option is `"1"` (`tmux show-options -wv -t @<id> @mure-subagents-window`).
   - The subagents window contains exactly 2 panes (`list-panes -t @<id>` returns 2 lines).
   - The originally-active window is still active (verifies `-d` actually kept the user where they were).
5. **Worker-spawn coverage (PRD §9.6).** From inside one of the spawned panes (use `tmux send-keys -t <pane_id> ... Enter` to run `mure spawn dummy` in that pane), trigger a third spawn. Poll `list-panes -s -t <session>` until the subagents window contains 3 panes (bounded retry loop with timeout — same shape as the existing harness's `waitFor` helper if present, else a `for i:=0;i<50;i++ { ...; time.Sleep(100*time.Millisecond) }`). Assert the third pane landed in the **same** subagents window (window count remained 2; no new window created).
6. Set `@mure-spawn-target right-of-active` via `tmux set-option -g`, spawn once more, and assert the spawn lands as a pane split in the original window (window count is still 2, original window's pane count grew by 1). This guards the no-regression path.

Reuse the existing `tmuxOut` and `mureCmd` helpers — no new harness machinery.

### Implementation checklist

- [x] Extend `cmd/mure/spawn_target.go` with `resolveSessionID`, `findSubagentsWindow`, and the new `subagents-window` / unknown-target branches. Import `os`.
- [x] Edit `cmd/mure/spawn.go`: change the `target=""` fallback as described (drop the right-of-active assignment).
- [x] Rename and update the empty-target test; add Tests 1, 2, 3, 4, 9, 10, 11 to `spawn_target_test.go`.
- [x] Add `TestEndToEndSubagentsWindow` to `test/e2e/e2e_test.go`.
- [x] Run `gofmt -w` on all touched files.

### Autonomous verification

```bash
cd /Users/blink/Code/mure
gofmt -l cmd/mure/spawn.go cmd/mure/spawn_target.go cmd/mure/spawn_target_test.go test/e2e/e2e_test.go
# Expected: empty.
go build ./...
go vet ./...
go test ./cmd/mure/... -count=1 -run . -v 2>&1 | tee /tmp/phase2-unit.log
grep -c '^--- PASS: TestPickSpawnTarget' /tmp/phase2-unit.log
# Expected: 11 total = 3 carried from Phase 1 (RightOfActive, BelowActive, NewWindow) + 1 renamed from Phase 1 (EmptyDefaultsToSubagentsWindow) + 7 new in Phase 2 (subagents missing, marker present, name-only, marker-beats-name, unknown target, TMUX_PANE set, TMUX_PANE unset).
go test -tags e2e ./test/e2e/... -count=1 -run TestEndToEndSubagentsWindow -v 2>&1 | tee /tmp/phase2-e2e.log
grep -E '^(--- PASS|--- FAIL|PASS|FAIL|ok|SKIP)' /tmp/phase2-e2e.log
# Expected: a --- PASS: TestEndToEndSubagentsWindow line. If the line says SKIP (tmux missing on this host), the phase is NOT complete — re-run on a host with tmux.
# Also re-run the pre-existing e2e suite to confirm no regressions:
go test -tags e2e ./test/e2e/... -count=1 -run TestEndToEnd$ -v 2>&1 | tail -20
# Expected: --- PASS: TestEndToEnd.
go test ./... -count=1 2>&1 | tail -30
# Expected: all non-e2e packages green. (This run does NOT include test/e2e; that's covered by the -tags e2e invocations above.)
```

Phase passes when: `gofmt` and `go vet` clean; all unit cases pass; both `TestEndToEndSubagentsWindow` and the pre-existing `TestEndToEnd` pass against real tmux (neither skipped); full-repo non-tagged `go test` green. If either e2e test skips for lack of tmux, the phase is **not** complete — surface to a human or run on a tmux-capable host.

---

## Phase 3: Documentation updates
**Depends on:** Phase 2

Reflect the new default and value in user-facing docs. Code changes already shipped in Phase 2; this phase touches only `.md` files.

### Scope

- [x] `README.md` — locate the tmux plugin options table (search for `@mure-spawn-target`). Change the default-value cell from `right-of-active` to `subagents-window`. If the table lists accepted values, add `subagents-window` and mark it as the default.
- [x] `tmux-mure/README.md` — same option table. Replace the row's description with:
  ```
  | `@mure-spawn-target` | `subagents-window` | Read by `mure spawn`: `subagents-window`, `right-of-active`, `below-active`, `new-window`. Unknown values warn and fall back to `subagents-window`. |
  ```
  (Preserve the surrounding column structure — copy the existing header row's column widths.)
- [x] `specs/001-init/PRD.md` — three edits per PRD §7.3:
  - Table row at line 249: update default and the accepted-values list to include `subagents-window` as the new default.
  - Line 387: update the "default when unset" sentence to say `subagents-window`.
  - The `mure spawn` row at line 204: update the stated default to `subagents-window`.
  Re-read the file before editing to confirm the line numbers (the PRD captured them at draft time and they may have drifted).

### Implementation checklist

- [x] Edit `README.md` (one cell).
- [x] Edit `tmux-mure/README.md` (one row).
- [x] Edit `specs/001-init/PRD.md` (three locations; verify line numbers first).
- [x] No code changes in this phase.

### Autonomous verification

```bash
cd /Users/blink/Code/mure
# Every doc that mentions @mure-spawn-target must now mention the new default.
for f in README.md tmux-mure/README.md specs/001-init/PRD.md; do
  grep -n '@mure-spawn-target' "$f" || { echo "MISSING in $f"; exit 1; }
done
# The new default value must appear in all three.
for f in README.md tmux-mure/README.md specs/001-init/PRD.md; do
  grep -q 'subagents-window' "$f" || { echo "subagents-window not mentioned in $f"; exit 1; }
done
# Each doc must now present subagents-window as the default in the @mure-spawn-target row.
# Use a positive check against the actual table cell content rather than a fragile negative regex.
for f in README.md tmux-mure/README.md; do
  grep -E '@mure-spawn-target.*subagents-window' "$f" >/dev/null \
    || { echo "$f: @mure-spawn-target row does not show subagents-window"; exit 1; }
done
# specs/001-init/PRD.md: the default-when-unset sentence must say subagents-window.
grep -E 'default.*subagents-window|subagents-window.*default' specs/001-init/PRD.md >/dev/null \
  || { echo "specs/001-init/PRD.md: default not updated to subagents-window"; exit 1; }
echo DOCS_OK
# Sanity: re-run tests to confirm no accidental code edits slipped in.
go test ./... -count=1 >/dev/null && echo TESTS_OK
```

Phase passes when: all three docs reference `subagents-window`, none labels `right-of-active` as the default, and tests remain green.

---

## Cross-phase invariants (asserted in every phase's verification)

- `go build ./...` and `go vet ./...` clean.
- `gofmt -l` returns empty for every touched file.
- All pre-existing tests pass unchanged — PRD §9.4. This includes `test/e2e/e2e_test.go::TestEndToEnd` (invoked with `-tags e2e` since the file is build-tagged).
- No new Go module dependencies (`go.mod` unchanged vs. plan start).
- Files outside the list in PRD §8 are not modified.
