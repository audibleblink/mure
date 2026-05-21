# Execution Plan — PRD 005: Generalize Harness Support

Source spec: `specs/005-generalize-harness/PRD.md`

Each phase below ends with a fully working, tested chunk of mure. Phases are designed so the tree builds and existing tests pass at every phase boundary. The hard cutover (deleting `pi-mure/` and `internal/piext`) is deferred to the final phase so earlier phases can land independently.

## Chunk 1: Foundations + Registry + CLI

## Phase 1: Harness registry package (embed + manifest loader)
**Depends on:** none

Stand up `internal/harnesses/` as the single Go package that knows about harnesses. No CLI wiring yet. This phase is pure library code with table tests.

### Tasks
- [x] Create empty `harnesses/` directory at repo root with a `.gitkeep` so `go:embed` has a target. No on-disk test fixtures; Phase 1 tests use `fstest.MapFS` exclusively against the registry's `fs.FS` injection point.
- [x] Add dependency `github.com/pelletier/go-toml/v2` to `go.mod`; run `go mod tidy`.
- [x] `internal/harnesses/manifest.go`: define `Manifest` struct mirroring §7 schema (Name, Display, Command, TaskArg, Capabilities{Spawn,Status,Result,Subtools}, Install.Skill{Path,Merge}, Install.Hooks[]{Src,Dst,Mode}). Add `ManifestVersion` reserved field. Use strict decode (`toml.Unmarshal` with `DisallowUnknownFields`); unknown keys error.
- [x] `internal/harnesses/taskarg.go`: parse `task_arg` strings into a typed value (`Positional|Stdin|Flag{Name}|None`). Invalid strings error.
- [x] `internal/harnesses/embed.go`: `//go:embed all:../../harnesses` FS. Export `FS()` accessor. (Implemented via sibling `harnesses` package — `go:embed` cannot escape its source dir with `..`.)
- [x] `internal/harnesses/registry.go`: `Load(fs.FS) ([]Manifest, error)` walks `<name>/manifest.toml`, returns sorted-by-name slice. `Get(name)` helper. Errors aggregate per-harness with manifest path in the message.
- [x] `internal/harnesses/manifest_test.go`: table tests covering: valid manifest, unknown key rejected, each `task_arg` variant, invalid `task_arg`, invalid merge mode, missing required fields (name, command).
- [x] `internal/harnesses/registry_test.go`: load from an in-memory `fstest.MapFS` with 2 harnesses; assert sort order, Get hit/miss.

### Verification (autonomous)
- [x] `go build ./...` succeeds.
- [x] `go test ./internal/harnesses/...` is green.
- [x] `go vet ./...` clean.

---

## Phase 2: Install planner + state file + `mure integration` rewrite
**Depends on:** Phase 1

Drive `mure integration {install,uninstall,list}` entirely from the registry. The existing pi-specific integration code stays in place until Phase 5; this phase replaces `cmd/mure/integration.go`'s internals with the registry-driven flow but leaves `pi-mure/` untouched.

### Tasks
- [x] `internal/harnesses/plan.go`: `type FileOp { Dst string; Mode fs.FileMode; Content []byte; Merge string }`. `BuildPlan(m Manifest, root fs.FS) ([]FileOp, error)` materializes skill + hooks into FileOps, expanding `~` via `os.UserHomeDir()`.
- [x] `internal/harnesses/markers.go`: append-block helpers — `WrapBlock(harness, body)` produces `# >>> mure:<harness> >>>` … `# <<< mure:<harness> <<<`, `ReplaceOrAppendBlock(existing, harness, body)` is pure-string idempotent replace, `StripBlock(existing, harness)` removes it. Unit-tested in isolation.
- [x] `internal/harnesses/apply.go`:
  - `Apply(ops []FileOp) (Receipt, error)` writes files with declared modes. `merge="append"` delegates to `markers.ReplaceOrAppendBlock`. `replace` overwrites. `create-if-missing` no-ops if dst exists.
  - `Receipt` records each Dst, its mode, sha256 of written content, and merge mode used.
- [x] State file: `internal/harnesses/state.go` reads/writes `$XDG_STATE_HOME/mure/integrations/<name>.json` (fallback `~/.local/state/mure/...` — macOS commonly has XDG vars unset, so fallback path must be unit-tested). Stores last `Receipt`.
- [x] `internal/harnesses/uninstall.go`: from receipt, delete `replace`/`create-if-missing` files whose hash still matches the receipt (skip-with-warn otherwise); for `append`, strip the marker block. Idempotent.
- [x] Rewrite `cmd/mure/integration.go`:
  - `mure integration list`: table with name, display, installed?, capability matrix; mark `status=false`/`result=false` as `degraded`.
  - `mure integration install <name>`: BuildPlan → Apply → write state. No-op when receipt matches.
  - `mure integration uninstall <name>`: load state → reverse → clear state.
- [x] Tests:
  - `internal/harnesses/apply_test.go`: each merge mode, idempotency (apply twice → identical filesystem), append marker round-trip with surrounding user content preserved.
  - `internal/harnesses/uninstall_test.go`: install→uninstall returns filesystem to original; modified-by-user files in `replace` mode are left and warned.
  - `cmd/mure/integration_test.go` (or extend existing): exec the subcommand against a `t.TempDir()` HOME with a synthetic embed FS via a test-only injection point on the registry (`internal/harnesses.SetFSForTesting`).

### Verification
- [x] `go test ./...` green.
- [x] Manual scripted check in CI: `HOME=$(mktemp -d) ./mure integration install _test && ./mure integration list | grep -q _test && ./mure integration install _test && ./mure integration uninstall _test` exits 0 and leaves HOME clean — wrapped as `test/harness_install.sh`.

---

## Phase 3: `mure emit` subcommand + daemon ingestion
**Depends on:** Phase 1

Add `mure emit` as the canonical NDJSON producer. Daemon wire format is unchanged.

### Tasks
- [ ] `cmd/mure/emit.go`:
  - `mure emit status <working|blocked|idle> [--tool <name>]`
  - `mure emit result -` (reads stdin until EOF)
  - Resolves pane/session/agent IDs from env (`MURE_SOCKET`, `MURE_PANE_ID`, `MURE_SESSION`, `MURE_AGENT_ID`); on missing env, falls back to `tmux display -p '#{pane_id}'` etc.; errors loudly if `MURE_SOCKET` is unset.
  - Marshals to the existing NDJSON frame shape used by `internal/daemon` and writes one line per invocation.
- [ ] Daemon schema decision: read `internal/daemon` frame definitions; if `status` lacks an optional `tool` field, add it (additive only — no renames). Decision recorded as a code comment in `cmd/mure/emit.go` referencing the frame type used. Binary outcome: either no daemon change, or one additive field.
- [ ] Register `emit` in `cmd/mure/main.go`.
- [ ] Tests:
  - `cmd/mure/emit_test.go`: spawn a unix-socket fake daemon in a goroutine, set `MURE_SOCKET`, invoke each subcommand, assert exact NDJSON bytes received.
  - Negative test: unset `MURE_SOCKET` → non-zero exit + clear error.

### Verification
- [ ] `go test ./cmd/mure/... ./internal/daemon/...` green.
- [ ] `test/emit_e2e.sh`: starts real `mure` daemon, runs `mure emit status working --tool foo`, then `mure ls --json` (or equivalent inspection) shows the status frame. Script asserts via `jq` and exits non-zero on mismatch.

---

## Chunk 2: Spawn wiring, fallback, migration, docs

## Phase 4: Harness-aware `mure spawn` + tmux session option + capture-pane fallback
**Depends on:** Phase 1, Phase 3

`mure spawn` consults the registry. Existing pi-only flow becomes one path through the same code.

### Tasks
- [ ] `cmd/mure/spawn.go`:
  - Add `--harness <name>` flag.
  - Resolution order: flag → `MURE_HARNESS` → `tmux show-option -qv @mure-harness` (session) → error listing all four slots.
  - From manifest: pick `command` and `task_arg`. Build argv/stdin per `task_arg` variant.
  - Export into spawned pane env: `MURE_PANE_ID`, `MURE_SESSION`, `MURE_AGENT_ID`, `MURE_SOCKET`, `MURE_HARNESS`.
- [ ] Pane→harness binding: at spawn time, `cmd/mure/spawn.go` sends a registration frame to the daemon recording `pane_id → harness_name`. Daemon stores this in its in-memory pane table so capability lookup is possible later. New task — not implicit.
- [ ] `internal/harnesses/fallback.go`: capture-pane status heuristic — pane is `idle` if `tmux capture-pane -p -t <pane>` output is byte-identical across two samples ≥ N seconds apart (N configurable, default 3s); else `working`. Sampling is driven by a **daemon-side poll loop** that ticks every 3s over panes whose recorded harness has `capabilities.status=false`. Result fallback: on `mure wait`, the daemon returns the last 200 lines of captured buffer with `degraded:true` in the response payload.
- [ ] Fallback lives in `internal/daemon` (server-side), not in `cmd/mure/wait.go`, so the sidebar and `mure wait` see the same data. `cmd/mure/wait.go` simply surfaces the `degraded` flag returned by the daemon.
- [ ] `mure integration list` already labels degraded harnesses (Phase 2); confirm consistency.
- [ ] Tests:
  - `cmd/mure/spawn_test.go`: table-driven resolution precedence using fakes for env and tmux; assert error message lists all four slots when nothing is set. Also asserts the registration frame for `pane_id→harness` is sent on spawn.
  - `cmd/mure/spawn_target_test.go`: extend to assert env vars are propagated.
  - `internal/harnesses/fallback_test.go`: mock `tmux capture-pane` via an injectable command runner; assert idle vs working classification and degraded result shape.

### Verification
- [ ] `go test ./...` green.
- [ ] `test/spawn_e2e.sh`: inside a throwaway tmux server, install a fake harness whose `command` is `bash -c 'echo READY; sleep 5'`, run `mure spawn --harness fake role`, grep new pane's env for `MURE_PANE_ID`, send `mure emit status working --tool foo`, then assert via `mure ls --json | jq '.panes[] | select(.pane_id==env.PANE) | .status'` equals `working` and `.tool` equals `foo`. (No TUI scraping.) Script exits non-zero on any mismatch.

---

## Phase 5: Ship `pi`, `claude`, `opencode` harnesses + delete `pi-mure/` and `internal/piext`
**Depends on:** Phase 2, Phase 4

Hard cutover. After this phase the only harness-aware Go package is `internal/harnesses`.

### Tasks
- [ ] **Pre-implementation verification step** (run first, output captured in PR description):
  - `claude --help` → confirm prompt-passing flag; pin exact `task_arg` value in manifest.
  - `opencode --help` and check opencode docs/repo → confirm instruction-file path and hook event names; pin exact values.
  - If any value differs from the assumptions below, update the manifest before writing hooks.
- [ ] `harnesses/pi/manifest.toml`: `command="pi"`, `task_arg` matches current pi spawn invocation (read from existing `cmd/mure/spawn.go` pre-deletion), `capabilities` all `true`. `install.skill.path` = pi skills dir (resolve from `pi-mure/README.md`); `install.hooks` enumerates the files below.
- [ ] `harnesses/pi/skill.md`: instructs the pi agent that `mure spawn` / `mure wait` exist as shell commands. Port relevant content from `pi-mure/README.md`.
- [ ] `harnesses/pi/hooks/` — port from `pi-mure/index.ts` with this explicit mapping:
  - `on-tool-start.sh` → `mure emit status working --tool "$TOOL"`
  - `on-tool-end.sh`   → `mure emit status idle`
  - `on-turn-end.sh`   → `mure emit result -` (reads pi's final message on stdin)
  - `on-blocked.sh`    → `mure emit status blocked` (if pi-mure had a blocked signal; omit otherwise — decide during port and note in PR)
- [ ] `harnesses/claude/manifest.toml`: `command="claude"`, `task_arg="flag:--prompt"` (pinned by verification step above), capabilities all `true`. `install.skill.path="~/.claude/CLAUDE.md"`, `merge="append"`. `install.hooks` enumerates each file below with `dst` under `~/.claude/hooks/`.
- [ ] `harnesses/claude/skill.md`: teaches the agent to shell out to `mure spawn` / `mure wait`.
- [ ] `harnesses/claude/hooks/` — file-by-file (Claude Code hook events):
  - `pre-tool-use.sh`  → `mure emit status working --tool "$CLAUDE_TOOL_NAME"`
  - `post-tool-use.sh` → `mure emit status idle`
  - `stop.sh`          → `mure emit result -` (reads final assistant message from the hook payload via stdin)
- [ ] `harnesses/opencode/manifest.toml`: `command="opencode"`, `task_arg` and `install.skill.path` pinned by verification step. `merge="append"`. Capabilities all `true` if opencode exposes the equivalent hook events; else set the missing capability to `false` and rely on capture-pane fallback (Phase 4).
- [ ] `harnesses/opencode/skill.md`: same content as claude's, adapted to opencode's instruction format.
- [ ] `harnesses/opencode/hooks/` — analogous tool-start / tool-end / turn-end scripts, mapped to opencode's actual event names from the verification step. Each script body is one `mure emit …` call.
- [ ] Delete `pi-mure/` directory and `internal/piext/` package. Remove their references from `cmd/mure/main.go`, `Makefile`, `go.mod` (if `pi-mure/` had its own module bits), and `README.md`.

- [ ] Manifest validation runs in CI: a `go test` in `internal/harnesses` iterates the embedded FS and asserts every shipped manifest decodes strictly.
- [ ] Update `README.md`:
  - Replace any `pi-mure` references.
  - Add "Adding a harness" section (≤1 page): create folder, write manifest, drop hooks, open PR. Reference §6/§7 of PRD.

### Verification
- [ ] `go build ./...` succeeds (no orphaned imports from deleted packages).
- [ ] `go test ./...` green.
- [ ] `mure integration list` shows exactly `pi`, `claude`, `opencode`.
- [ ] `test/acceptance.sh` is added to CI (not just runnable locally) and runs every numbered item in PRD §14, exiting 0:
  - manifests validate
  - list output matches expected
  - install pi → install pi (no-op) → uninstall pi leaves HOME clean
  - `mure spawn` with no resolution source prints the 4-slot error
  - `mure spawn --harness claude` (with `claude` shimmed by a stub on PATH) launches and env-propagates
  - hook-driven status/result reflect in sidebar / `mure wait`
  - a synthetic `capabilities.status=false` harness exercises the capture-pane fallback and is labeled degraded
  - `pi-mure/` and `internal/piext/` absent from tree (`! test -e pi-mure && ! test -e internal/piext`)
  - README contains an "Adding a harness" section
