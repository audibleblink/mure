# mure — Execution Plan

Phases are sequenced so that after each one, the repo builds green, tests pass, and a discrete, demoable slice of mure exists. Dependencies form a DAG; the `Depends on` line under each heading is authoritative for `sdd-apply`.

Conventions:

- Every phase ends with an **autonomous verification loop** (a command, script, or test target an agent can run unattended and read a pass/fail signal from).
- Code lives where the PRD dictates (`cmd/`, `internal/...`, `pi-mure/`, `tmux-mure/`). No new top-level directories.
- "Done" means: code compiles, lints clean, new tests pass, and prior phases' tests still pass.

---

## Chunk 1: Foundations (scaffolding, contracts, daemon core)

## Phase 1: Repository scaffolding & build toolchain
**Depends on:** none

Stand up the monorepo skeleton so every later phase has a place to drop code and a working `make build` / `make test` loop.

Tasks:

- [x] Initialize Go module at repo root: `go mod init github.com/<owner>/mure` (Go 1.22).
- [x] Create directory tree exactly as in PRD §4 (`cmd/mure/`, `internal/{daemon,tmuxctl,sock,sidebar,piext/assets}/`, `pi-mure/`, `tmux-mure/scripts/`, `.github/workflows/`).
- [x] Add a stub `cmd/mure/main.go` that prints `mure (placeholder)` and exits 0, so the module builds.
- [x] Add `Makefile` with targets:
  - `sync-piext` → `rsync -a --delete pi-mure/ internal/piext/assets/` (create empty `pi-mure/.keep` so rsync has a source).
  - `build` → depends on `sync-piext`, runs `go build -o bin/mure ./cmd/mure`.
  - `test` → `go test ./...` then `shellcheck tmux-mure/scripts/*.sh tmux-mure/tmux-mure.tmux` (skip gracefully if shellcheck absent in early phases).
  - `lint` → `go vet ./...`, `gofmt -l .` (fail if non-empty).
  - `verify` → `sync-piext` + `build` + `lint` + `test`.
- [x] Add `.github/workflows/ci.yml` with two jobs (`ubuntu-latest`, `macos-latest`) each running `make verify`. Cache Go modules.
- [x] Add `.gitignore` (bin/, internal/piext/assets/, *.test, coverage.out).
- [x] Add `README.md` with one-paragraph project description and pointer to `prd.md`.
- [x] Commit `pi-mure/.keep` and `tmux-mure/.keep` placeholder files so rsync + workflows don't fail before later phases populate them.

Verification loop:

- [x] `make verify` exits 0 locally.
- [x] CI run on a throwaway branch shows both OS jobs green.
- [x] `./bin/mure` prints `mure (placeholder)` with exit 0.

---

## Phase 2: Socket protocol types & NDJSON framing (`internal/sock`)
**Depends on:** Phase 1

Define the wire contract from PRD §12 as Go types, with framing helpers, so daemon and CLI hook code can share types via internal import.

Tasks:

- [x] Create `internal/sock/types.go` with structs for every frame in §12: `Hello`, `Status`, `Bye`, `Focus`, `PaneDied`, `SessionClosed`, `Roster`, `AgentUpdate`. Each carries `V int` (json tag `v`) and `Event string`.
- [x] Define `Status` enum constants (`StatusIdle`, `StatusWorking`, `StatusBlocked`, `StatusDisconnected`, `StatusErrored`) and a `ValidStatus(s string) bool`.
- [x] Define `Role` constants (`RoleAgent`, `RoleSidebar`, `RoleCLI`, `RoleHook`).
- [x] Create `internal/sock/frame.go` with:
  - `WriteFrame(w io.Writer, v any) error` — marshals JSON, appends `\n`, single `Write` call.
  - `ReadFrame(r *bufio.Reader, max int) ([]byte, error)` — reads one `\n`-terminated line, errors if >max (default 64KB per §12).
  - `DecodeEnvelope([]byte) (event string, version int, err error)` — peek for routing before full decode.
- [x] Constants: `ProtocolVersion = 1`, `MaxFrameSize = 64 * 1024`.
- [x] Unit tests `internal/sock/frame_test.go`:
  - Round-trip every frame type from §12 verbatim (use the literal JSON examples from the PRD as golden inputs).
  - `ReadFrame` rejects frames > 64KB with a sentinel error.
  - `DecodeEnvelope` extracts event name without full unmarshal.
  - Rejects frames with `v != 1`.

Verification loop:

- [x] `go test ./internal/sock/...` passes.
- [x] `make verify` still green.

---

## Phase 3: tmux control-mode client (`internal/tmuxctl`)
**Depends on:** Phase 1

Isolate everything that speaks `tmux -C` behind an interface so the daemon can be unit-tested against a scripted fake.

Tasks:

- [x] Create `internal/tmuxctl/client.go` defining:
  ```go
  type Client interface {
      Run(ctx context.Context, cmd string) (string, error) // synchronous reply via %begin/%end
      Events() <-chan Event                                // async % notifications
      Close() error
  }
  ```
- [x] Define `Event` as a tagged union (struct with `Kind` + payload fields) covering `%window-add`, `%window-close`, `%pane-died`, `%session-window-changed`, `%layout-change`, `%output` (suppressed, but parsed if seen), `%begin`/`%end`/`%error` for reply correlation.
- [x] Implement `realClient` that spawns `tmux -C attach -t <session>` (or `new-session`) with two goroutines:
  - Reader: line-oriented scanner; routes `%begin`/`%end`/`%error` to per-request reply channels (FIFO queue), routes other `%` lines to `Events()`.
  - Writer: serialized via a request channel; one in-flight command at a time (no pipelining, per PRD §6.1).
- [x] Implement `scriptedFake` in `internal/tmuxctl/fake.go` exposing `EnqueueReply`, `EmitEvent`, used by tests.
- [x] Add `internal/tmuxctl/parse.go` for `%` line parsing with unit tests covering each event type using canned tmux output captured by hand (commit as `testdata/*.txt`).
- [x] Tests:
  - Parser table tests for every event kind.
  - Reply correlator test: enqueue two commands, fake emits `%begin 1` … `%end 1` then `%begin 2` … `%end 2`, both `Run` calls return correct output.
  - Error reply: `%error` resolves the in-flight `Run` with a Go error.

Verification loop:

- [x] `go test ./internal/tmuxctl/...` passes (no real tmux required; uses fake + parser fixtures).
- [x] `make verify` green.

---

## Phase 4: Daemon roster, socket server, peer-auth
**Depends on:** Phase 2, Phase 3

Build the daemon's in-memory state machine and Unix-socket front door — independent of any real tmux. After this phase, an agent stub can connect, send `hello`+`status`, and a sidebar stub can subscribe and see roster updates.

Tasks:

- [x] Create `internal/daemon/roster.go`:
  - `Roster` owned by a single goroutine; all mutations via a request channel (PRD §6.2).
  - Operations: `UpsertFromHello`, `ApplyStatus`, `MarkDisconnected(agentID, after time.Duration)`, `MarkErrored`, `Remove`, `Snapshot()`.
  - Subscriber API: `Subscribe() (<-chan sock.AgentUpdate, cancel func())` with bounded 64-frame buffer per subscriber; overflow closes the channel (PRD §9 backpressure).
- [x] Create `internal/daemon/server.go`:
  - `Serve(ctx, socketPath)` — listens on Unix socket, sets `0700` parent dir and `0600` socket mode explicitly.
  - On accept, calls `peerauth.Check(conn)` (next bullet) and rejects non-self UIDs.
  - First frame must be `hello`; dispatch by role to `handleAgent`, `handleSidebar`, `handleHook`, `handleCLI`.
  - Stale-socket cleanup on bind failure: try `hello` ping with 200ms timeout, unlink if no response (PRD §6.4).
- [x] Create `internal/daemon/peerauth_linux.go` and `peerauth_darwin.go` using `SO_PEERCRED` / `getpeereid` via `golang.org/x/sys/unix`. Build-tag separated. Reject FreeBSD with a compile-time-friendly stub returning "unsupported".
- [x] Implement EPIPE-vs-pane-died debounce in `internal/daemon/debounce.go`: 1s timer per agent; if `pane_died` arrives in window → `errored`, else → `disconnected` (PRD §6.3). Window stretches if reader lag detected (expose a `Stretch(d)` hook for the tmux reader goroutine to call).
- [x] Implement coalescer in `internal/daemon/coalesce.go`: per-(paneID, option) 500ms debounce → emits writes onto an output channel. This phase wires it to an in-memory sink; Phase 5 plugs it into the real tmux writer.
- [x] Tests:
  - Roster: concurrent upserts serialized; subscriber receives expected sequence; overflow disconnects slow subscriber.
  - Server: scripted client connects, sends `hello{role=agent}` + `status`, server emits matching `agent_update` to a subscribed sidebar stub.
  - Server: non-`hello` first frame → connection closed.
  - Server: oversized frame → connection closed.
  - Stale-socket cleanup: pre-existing socket file with no listener gets unlinked; with a listening peer that responds, second daemon refuses to start.
  - Peer auth: a connection from a faked different UID (via `net.Pipe` + injected mock) is rejected.
  - Debounce: EPIPE without pane_died → `disconnected` after 1s; EPIPE then pane_died within 500ms → `errored`.
  - Coalescer: 5 writes to same (pane, option) within 500ms → one emission with the last value.

Verification loop:

- [x] `go test ./internal/daemon/...` passes.
- [x] A small `internal/daemon/cmd_e2e_test.go` starts the server on a temp socket, spawns an in-process agent stub + sidebar stub, asserts roster propagation end-to-end — no real tmux involved.
- [x] `make verify` green.

---

## Phase 5: Daemon ↔ tmux integration (control mode, pane options)
**Depends on:** Phase 3, Phase 4

Glue the daemon to a real tmux server via the `tmuxctl.Client` interface, applying pane-option writes from the coalescer and consuming `%` events into roster transitions.

Tasks:

- [x] Create `internal/daemon/tmuxbridge.go`:
  - Owns two `tmuxctl.Client`s (reader + writer) per PRD §6.1.
  - Reader issues `refresh-client -f no-output` on attach.
  - Translates `%pane-died` → roster `MarkErrored` and feeds the debouncer.
  - Translates `%window-close` / `%session-window-changed` into roster removals.
  - Writer consumes coalescer output: `set-option -p -t <pane_id> @mure-<key> <value>`.
- [x] Session env setup helper: when daemon starts, runs `set-environment -t <session> MURE_ENV 1`, `MURE_SESSION`, `MURE_RUN_DIR`, `MURE_SOCKET`. Per-pane `MURE_AGENT_ID` is set by `mure spawn` in Phase 6.
- [x] Add `internal/daemon/daemon.go` exposing `Run(ctx, Config) error` that wires: tmuxbridge + socket server + roster + coalescer + logger.
- [x] Logger: `internal/daemon/log.go` writes to `$MURE_RUN_DIR/daemon.log`, rotates at 4MB keeping one `.1` (PRD §6.6).
- [x] Runtime-dir resolver `internal/daemon/paths.go`: Linux `$XDG_RUNTIME_DIR/mure/<session>/`, macOS `~/Library/Caches/mure/<session>/` (PRD §15).
- [x] Tests:
  - `tmuxbridge_test.go` driven by `scriptedFake`: emit `%pane-died %41`, expect agent for `%41` to become `errored` within debounce window.
  - Coalescer→writer test: roster transitions to `working` cause exactly one `set-option -p -t %41 @mure-status working` within 500ms.
  - Log rotation test: write >4MB, assert `.log` truncated and `.log.1` exists.
- [x] Integration test `internal/daemon/integration_real_tmux_test.go` guarded by `//go:build integration`:
  - Starts a real `tmux -L mure-test new-session -d -s test`.
  - Starts daemon against it.
  - In-process agent stub connects, sends `status: working, task: "x"`.
  - Test asserts `tmux show-options -pv -t <pane> @mure-status` returns `working` within 1s.
  - Tears down tmux server.

Verification loop:

- [x] `go test ./internal/daemon/...` passes (unit).
- [x] `go test -tags=integration ./internal/daemon/...` passes on a host with tmux ≥3.2 installed; CI runs this on the `ubuntu-latest` and `macos-latest` jobs (install tmux via apt / brew in workflow).
- [x] `make verify` green.

---

## Chunk 2: CLI, sidebar, pi extension, tmux plugin, end-to-end

## Phase 6: CLI surface (`cmd/mure`)
**Depends on:** Phase 5

Wire every CLI verb in PRD §7. After this phase, a human (or test) can run `mure up`, `mure spawn`, `mure ls`, `mure focus`, `mure down`, `mure _hook`, `mure doctor`, `mure integration {install,uninstall} pi`.

Tasks:

- [x] Replace `cmd/mure/main.go` with a dispatcher (stdlib `flag`, no Cobra unless trivially small). One file per verb under `cmd/mure/`:
  - `up.go` — start daemon (forks self with `MURE_DAEMON=1`); re-entrant: pings socket first and exits 0 with `already running` if healthy (PRD §6.5). Warns if `tmux-mure` plugin missing (`tmux show-option -gv @mure-plugin-version` empty).
  - `down.go` — connect, send `cli` shutdown frame; daemon unlinks socket on exit.
  - `ls.go` — connect as `cli`, request a one-shot roster snapshot, render table; `--json` flag dumps raw.
  - `spawn.go` — read `@mure-spawn-target` (default `right-of-active`), run `tmux split-window` (or `new-window`) executing `$MURE_AGENT_CMD` (default `pi`) with `MURE_ENV=1`, `MURE_AGENT_ID=agent-<n>`, `MURE_SOCKET=…`; then `set-option -p` for `@mure-role`, `@mure-spawned-at`.
  - `focus.go` — `tmux select-pane -t <pane>` after resolving agent id.
  - `hook.go` — `mure _hook <event> <args>`: opens socket, writes one frame (`focus`/`pane_died`/`session_closed`) per PRD §12.2, exits.
  - `doctor.go` — checks tmux ≥3.2 (`tmux -V`), plugin presence, socket path writable, peer-auth syscall available (compile-time stub check), prints actionable hints.
  - `integration.go` — `install pi`: walks `internal/piext/assets/` (embedded via `//go:embed`) and writes to `$PI_CODING_AGENT_DIR/extensions/mure/` (default `~/.pi/agent/extensions/mure/`). `uninstall pi`: removes that dir.
  - `sidebar.go` — invokes `internal/sidebar.Run()` (stub until Phase 7; for now print "not yet implemented").
- [x] Create `internal/piext/embed.go` with `//go:embed assets/*` exposing `fs.FS`. Build will sync first via Makefile.
- [x] Tests `cmd/mure/*_test.go`:
  - Each verb has a unit test that runs against a daemon started in-process on a temp socket (reuse Phase 4 helpers). Verifies exit code and output format.
  - `up` re-entrancy: second invocation returns `already running` exit 0.
  - `doctor` on a host without tmux returns non-zero with helpful message (use `PATH` override in test).
  - `integration install pi` to a temp dir produces the expected file tree; `uninstall` removes it.
  - `_hook focus` sends exactly one well-formed frame and exits.
- [x] `ls --json` snapshot test against a roster fixture.

Verification loop:

- [x] `go test ./cmd/mure/...` passes.
- [x] `make verify` green.
- [x] Manual smoke (also scripted in `scripts/smoke.sh`): start a tmux session, `mure up`, `mure spawn dummy 'sleep 60'`, `mure ls` shows the new agent.

---

## Phase 7: Bubble Tea sidebar (`internal/sidebar`)
**Depends on:** Phase 4, Phase 6

Implement the sidebar TUI as in PRD §9. After this phase, `mure sidebar` inside an already-split pane renders live agent state.

Tasks:

- [x] Add deps: `github.com/charmbracelet/bubbletea`, `lipgloss`, `bubbles/list` (or hand-rolled list — pick whichever keeps code smaller).
- [x] `internal/sidebar/client.go` — connects to `$MURE_SOCKET`, sends `hello{role=sidebar}`, reads frames into a typed channel; reconnects with exponential backoff capped at 30s.
- [x] `internal/sidebar/model.go` — Bubble Tea `Model` holding roster, selection index, connection status. `Update` handles frames + key events; `View` renders the box from PRD §9 (illustrative glyphs: `●` working, `◐` blocked, `✓` idle-recent, `○` idle-stale, `⚠` errored, `⋯` disconnected).
- [x] Keybindings (sidebar pane only) per PRD §9:
  - `j`/`k` → move selection.
  - `↵` → shells out: `tmux select-pane -t <pane>`.
  - `q` → `tmux kill-pane -t $TMUX_PANE`.
- [x] On disconnect, View overlays `(disconnected)`; on reconnect, fresh `roster` repopulates.
- [x] Wire `cmd/mure/sidebar.go` to `internal/sidebar.Run(ctx)`.
- [x] Tests:
  - `model_test.go` drives the model with synthetic `roster`/`agent_update` messages and asserts `View()` output (snapshot tests via `golden.txt` files).
  - `client_test.go` runs against an in-process daemon stub: connect → receive roster → kill server → assert reconnect attempt → restart server → assert fresh roster received.

Verification loop:

- [x] `go test ./internal/sidebar/...` passes.
- [x] `make verify` green.
- [x] Optional manual: run `MURE_SOCKET=… mure sidebar` and confirm rendering; not required for CI gate (golden tests cover it).

---

## Phase 8: pi extension (`pi-mure/`)
**Depends on:** Phase 2

The TypeScript extension lives at `pi-mure/` and is the source of truth; `make sync-piext` copies it into `internal/piext/assets/`. Tests mock the socket.

Tasks:

- [x] `pi-mure/package.json` — name `@mure/pi-extension`, `type: "module"`, `private: true` (publishing deferred per PRD §4). Scripts: `test` → `node --test test/*.test.ts` (or `vitest` if smaller; pick one).
- [x] `pi-mure/tsconfig.json` — target ES2022, module NodeNext, strict.
- [x] `pi-mure/index.ts`:
  - Gate: return immediately unless `process.env.MURE_ENV === "1"` && `process.env.MURE_AGENT_ID`.
  - Connect to `process.env.MURE_SOCKET` via `net.createConnection`.
  - Send `hello` with `agent_id`, `pane_id = process.env.TMUX_PANE`, `pid = process.pid`, `pi_version` (from pi runtime API; if absent, `"unknown"`).
  - Subscribe to pi lifecycle events (PRD §10 step 3) and emit `status` frames accordingly.
  - Reconnect: exp backoff 250ms→30s with full jitter.
  - Outbound buffering during disconnect: keep a small queue; coalesce duplicate `status` events (replace the last buffered `status`); never drop `hello`/`bye`.
  - `session_shutdown` → send `bye`, end socket.
- [x] `pi-mure/test/`:
  - `frame.test.ts` — golden encode/decode against PRD §12 examples (must match Go side byte-for-byte).
  - `gate.test.ts` — without env vars, extension is a no-op (no socket attempted).
  - `lifecycle.test.ts` — fake pi event bus → expected outbound frame sequence.
  - `reconnect.test.ts` — server drops connection; extension reconnects; buffered `status` coalesces to the last one.
- [x] CI workflow job: set up Node 20, run `cd pi-mure && npm install && npm test`. Add a step before Go build: `make sync-piext` then `git diff --exit-code internal/piext/assets/` to verify devs committed the sync (PRD §4 "CI verifies the sync was committed").

Verification loop:

- [x] `cd pi-mure && npm test` passes.
- [x] `make sync-piext && git diff --exit-code internal/piext/assets/` is clean.
- [x] `make verify` green.

---

## Phase 9: tmux plugin (`tmux-mure/`)
**Depends on:** Phase 6

Pure shell + tmux config. Owns hooks, status-line snippet, pane decoration, sidebar toggle, spawn-target policy. Zero processes spawned by the plugin itself.

Tasks:

- [x] `tmux-mure/tmux-mure.tmux` (TPM entrypoint, executable):
  - Sets `@mure-plugin-version 1`.
  - Installs the three hooks from PRD §11.1 (after-select-pane, pane-exited, session-closed) with `command -v mure` guards.
  - Sets `pane-border-format` per §11.3.
  - Defines `@mure-status-format` per §11.2 (without touching `status-right`).
  - Default color options (`@mure-color-working` etc.).
  - Default `@mure-sidebar-width 36`, `@mure-sidebar-position left`, `@mure-sidebar-key M`, `@mure-spawn-target right-of-active`.
  - Binds `bind-key -T prefix M run-shell '<plugin_dir>/scripts/sidebar-toggle.sh'`, with key read from `@mure-sidebar-key`.
- [x] `tmux-mure/scripts/sidebar-toggle.sh`:
  - If a pane in current session has `@mure-is-sidebar=1` → `tmux kill-pane -t` that pane.
  - Else → `tmux split-window` using `-h`/`-v` + `-b` based on `@mure-sidebar-position`, width from `@mure-sidebar-width`, command `mure sidebar`; then `tmux set-option -p @mure-is-sidebar 1` on the new pane.
- [x] `tmux-mure/scripts/uninstall-hooks.sh`: `set-hook -gu` for each hook installed.
- [x] `tmux-mure/example.tmux.conf` — recommended user snippet (the `status-right` append, `@plugin` line).
- [x] `tmux-mure/README.md` — install (TPM line), uninstall, configurable options table.
- [x] Lint: `shellcheck` clean over all `*.sh` and `tmux-mure.tmux`. Add `shellcheck` install step to CI (apt-get / brew).
- [x] Bats-style or plain bash integration test `tmux-mure/test/hooks_test.sh`:
  - Starts a tmux server in temp `TMUX_TMPDIR`, sources the plugin, runs `tmux display-message -p '#{hook}'`-style probes to confirm hooks are registered (`tmux show-hooks -g`).
  - Verifies sidebar-toggle creates and then destroys a pane with `@mure-is-sidebar=1` (uses a stub `mure` on PATH that just `sleep`s).
- [x] Add `make tmux-test` target running the above; include in `make verify`.

Verification loop:

- [x] `shellcheck tmux-mure/scripts/*.sh tmux-mure/tmux-mure.tmux` exits 0.
- [x] `make tmux-test` exits 0 on a host with tmux ≥3.2.
- [x] CI runs both on linux and macOS.
- [x] `make verify` green.

---

## Phase 10: End-to-end integration, throughput test, release wiring
**Depends on:** Phase 5, Phase 7, Phase 8, Phase 9

Final phase: prove all four contracts hold together against a real tmux + real pi-extension shape, plus the performance budget from PRD §13.

Tasks:

- [x] `test/e2e/e2e_test.go` (build tag `e2e`):
  - Spins up real `tmux` in a temp `TMUX_TMPDIR`.
  - Runs `mure up`.
  - Sources `tmux-mure/tmux-mure.tmux` into the test server.
  - Spawns three pseudo-agents via `mure spawn dummy <stub-agent-cmd>`, where `stub-agent-cmd` is a small Go binary built in `test/e2e/stubagent/` that speaks the socket protocol (mimicking the TS extension contract).
  - Asserts: `mure ls --json` shows three agents; each transitions `idle`→`working`→`idle` on cue; `@mure-status` pane option reflects each transition within 1s.
  - Toggles sidebar via prefix `M`; asserts a pane with `@mure-is-sidebar=1` exists.
  - `mure down`; asserts socket gone, pane options retained.
- [x] `test/throughput/throughput_test.go` (build tag `e2e`): 8 panes each running `yes`, daemon attached with `%output` suppressed; measure p99 latency from agent `status` frame send to pane-option write observable via `show-options`. Assert p99 < 100ms (PRD §13).
- [x] Cross-protocol golden test `test/protocol/golden_test.go`: load JSON fixtures used by both Go (`internal/sock`) and TS (`pi-mure/test`) and assert byte-identical encoding from each side (Go decodes, re-encodes, compares to fixture; TS test does same; CI cross-checks the fixture file is identical to the one used in `pi-mure/test/fixtures/`).
- [x] `mure doctor` end-to-end: run after a full install in CI; assert exit 0 with all checks green.
- [x] Release wiring:
  - GoReleaser config `.goreleaser.yaml` building darwin/{amd64,arm64} + linux/{amd64,arm64}, triggered on `mure-v*` tag.
  - Homebrew tap action (template only; tap repo out of scope) wired to publish on tag.
  - Document release flow in `README.md`: `mure-vX.Y.Z`, `pi-mure-vX.Y.Z`, `tmux-mure-vX.Y.Z`.
- [x] CI: add a third job `e2e` (linux + macOS) running `make verify && go test -tags=e2e ./test/...` with tmux installed.

Verification loop:

- [x] `go test -tags=e2e ./test/...` passes locally.
- [x] CI `e2e` job green on both OSes.
- [x] Throughput test p99 < 100ms threshold met (test fails otherwise).
- [x] `make verify` green; all prior phases' tests still pass.
- [x] Tag a `mure-v0.1.0-rc.1` in a fork; GoReleaser dry-run builds all four binaries.
