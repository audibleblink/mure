# Execution Plan — PRD 003: pi-mure Tools (`mure_spawn` & `mure_wait`)

Spec: `specs/003-pi-mure-tools/PRD.md`
Implementation surface: `pi-mure/index.ts` and a new `pi-mure/test/tools.test.ts`. No other files touched. No new `dependencies`.

## Resolved API contract (locked before planning)

Confirmed by reading `node_modules/@earendil-works/pi-coding-agent/dist/core/extensions/types.d.ts` (resolved via `~/.local/share/mise/installs/node/24.7.0/lib/node_modules/...`):

- Registration method: `pi.registerTool<TParams extends TSchema>(tool: ToolDefinition<TParams, TDetails, TState>): void`.
- `ToolDefinition` required fields used here:
  - `name: string`
  - `label: string` (human-readable; we use `"mure spawn"` / `"mure wait"`)
  - `description: string` (LLM-facing)
  - `parameters: TSchema` (TypeBox)
  - `execute(toolCallId, params, signal, onUpdate, ctx): Promise<AgentToolResult>`
  - Optional `promptGuidelines: string[]` — **note: array of strings**, not a single string (PRD §7.2/§8.2 phrasing collapses to a one-element array).
- TypeBox import: `import { Type } from "typebox"` (package id is literal `"typebox"`, re-exported via `pi-coding-agent`'s deps). Same for `type Static`.
- `AgentToolResult` (imported transitively; shape used here) is the standard pi tool result:
  ```ts
  { content: Array<{ type: "text"; text: string }>; isError?: boolean; details?: unknown }
  ```
  - **Success** → `{ content: [{ type: "text", text: <payload> }] }`.
  - **Error** → `{ isError: true, content: [{ type: "text", text: <message> }] }`. We do **not** throw from `execute`.
- Existing `pi-mure/index.ts` already imports `ExtensionAPI` from `"@earendil-works/pi-coding-agent"` and `tsconfig.json` has `skipLibCheck: true`; the package is resolvable at runtime from pi's global `node_modules`. No new `dependencies` entry needed (matches existing pattern).

These decisions are final for this plan. Tests assert against this exact `AgentToolResult` shape.

Phases are sized so each one leaves the package compiling, all tests green, and a coherent slice of behavior shipped. Each phase ends with an autonomous verification command.

---

## Chunk 1: Plan

## Phase 1: Exec abstraction + tool factories with unit tests
**Depends on:** none

Deliver the pure, injectable building blocks for both tools and cover behavior-level test cases (PRD §11 tests 1–10) without touching the live `mureExtension(pi)` registration path yet. At the end of this phase, the factories exist, are exported, and are exercised by tests using a stub `ExecFn`. Live registration is deferred to Phase 2 so this phase ships without changing observable extension behavior.

### Scope — additions to `pi-mure/index.ts`

All new code goes below the existing `start()`/`mureExtension()` block in a clearly delimited `// ── mure tools ─────────────────────` section. Names and shapes:

- `export type ExecFn` — exactly as PRD §9:
  ```ts
  export type ExecFn = (
    bin: string,
    argv: string[],
    opts: { env: NodeJS.ProcessEnv; timeoutMs?: number },
  ) => Promise<{ code: number; stdout: string; stderr: string; timedOut: boolean }>;
  ```
- `export const defaultExec: ExecFn` — wraps `child_process.execFile`. Implementation:
  - Call `execFile(bin, argv, { env: opts.env })` without using its built-in `timeout` option.
  - Buffer `stdout`/`stderr` from the returned child.
  - If `opts.timeoutMs` set, start `setTimeout`; on fire: `child.kill("SIGTERM")`, set a second `setTimeout(2000)` for `child.kill("SIGKILL")`, mark `timedOut = true`.
  - On child `"exit"` (or `"close"`): clear timers, resolve with `{ code: code ?? (signal ? -1 : 0), stdout, stderr, timedOut }`.
  - Never reject. Errors (`ENOENT` etc.) resolve as `{ code: -1, stdout: "", stderr: String(err), timedOut: false }`.
- `export function resolveMureBin(env: NodeJS.ProcessEnv): string` — returns `env.MURE_BIN` if truthy, else `"mure"`.
- `export function makeMureSpawnTool(exec: ExecFn, env: NodeJS.ProcessEnv): ToolDefinition` — returns:
  ```ts
  {
    name: "mure_spawn",
    label: "mure spawn",
    description: "Start a mure-managed coding-agent subagent in a new tmux pane. Pair with mure_wait to collect its result.",
    promptGuidelines: ["Use mure_spawn to start a coding-agent subagent in a new tmux pane. Pair with mure_wait to collect its result."],
    parameters: Type.Object({ role: Type.String({ minLength: 1 }), task: Type.Optional(Type.String()) }),
    async execute(_id, { role, task }) {
      const argv = task === undefined ? ["spawn", role] : ["spawn", role, task];
      const { code, stdout, stderr } = await exec(resolveMureBin(env), argv, { env });
      if (code !== 0) return errorResult(`mure spawn failed (exit ${code}): ${stderr.trim()}`);
      const parsed = parseSpawnStdout(stdout);
      if (!parsed) return errorResult(`mure_spawn: unparseable output: ${stdout}`);
      return okResult(JSON.stringify(parsed));
    },
  }
  ```
- `export function makeMureWaitTool(exec: ExecFn, env: NodeJS.ProcessEnv): ToolDefinition` — returns:
  ```ts
  {
    name: "mure_wait",
    label: "mure wait",
    description: "Block until a previously spawned mure agent emits its final result.",
    promptGuidelines: ["Use mure_wait to block until a previously spawned agent emits a result."],
    parameters: Type.Object({ agent_id: Type.String({ minLength: 1 }), timeout_ms: Type.Optional(Type.Integer({ minimum: 1 })) }),
    async execute(_id, { agent_id, timeout_ms }) {
      const ms = timeout_ms ?? 300_000;
      const { code, stdout, stderr, timedOut } = await exec(resolveMureBin(env), ["wait", agent_id], { env, timeoutMs: ms });
      if (timedOut) return errorResult(`mure_wait timed out after ${ms}ms`);
      if (code !== 0) return errorResult(`mure wait failed (exit ${code}): ${stderr.trim()}`);
      return okResult(stdout.endsWith("\n") ? stdout.slice(0, -1) : stdout);
    },
  }
  ```
- Internal helpers (not exported unless tests need them; export `parseSpawnStdout` so test 4 can be a unit assertion if convenient):
  - `okResult(text: string): AgentToolResult` → `{ content: [{ type: "text", text }] }`.
  - `errorResult(text: string): AgentToolResult` → `{ isError: true, content: [{ type: "text", text }] }`.
  - `parseSpawnStdout(stdout: string): { agent_id: string; pane_id: string } | null` per PRD §7.4: take the last non-empty line (split on `/\r?\n/`, filter `trim() !== ""`, take last), split on `/\s+/`, require exactly 2 tokens, require token 2 to start with `%`. Return `null` on any failure.

### Test file — new `pi-mure/test/tools.test.ts`

Use `node:test` + `node:assert/strict` (matches `gate.test.ts`). Provide a `makeStubExec()` helper:
```ts
function makeStubExec(canned: { code?: number; stdout?: string; stderr?: string; timedOut?: boolean } = {}) {
  const calls: Array<{ bin: string; argv: string[]; opts: { env: NodeJS.ProcessEnv; timeoutMs?: number } }> = [];
  const exec: ExecFn = async (bin, argv, opts) => {
    calls.push({ bin, argv, opts });
    return { code: 0, stdout: "", stderr: "", timedOut: false, ...canned };
  };
  return { exec, calls };
}
```
Tool result assertions look like:
```ts
const r = await tool.execute("id", { role: "planner" }, undefined, undefined, {} as any);
assert.equal(r.isError, undefined);
assert.deepEqual(JSON.parse(r.content[0].text), { agent_id: "agent-abc", pane_id: "%12" });
```
For error cases: `assert.equal(r.isError, true); assert.match(r.content[0].text, /substr/);`.

Tests to author (one-to-one with PRD §11 cases 1–10):

- [x] **Test 1** spawn happy path (no task) — `argv === ["spawn","planner"]`; stdout `"agent-abc %12\n"`; success result parses to `{ agent_id: "agent-abc", pane_id: "%12" }`.
- [x] **Test 2** spawn happy path (with task) — task `"fix the flaky test"` → `argv === ["spawn","builder","fix the flaky test"]` (assert via `deepEqual`).
- [x] **Test 3** spawn honors `MURE_BIN` — when env has `MURE_BIN="/opt/mure/bin/mure"`, `calls[0].bin === "/opt/mure/bin/mure"`; with `MURE_BIN` unset, `calls[0].bin === "mure"`.
- [x] **Test 4** spawn parse failure — stdout `"weird banner\n"` → `isError: true`, message contains `"weird banner"`.
- [x] **Test 5** spawn non-zero exit — `code: 2`, stderr `"role unknown\n"` → `isError: true`, message contains both `"role unknown"` and `"2"`.
- [x] **Test 6** wait happy path — `argv === ["wait","agent-abc"]`; stdout `"final answer text\n"`; success `content[0].text === "final answer text"`.
- [x] **Test 7** wait default timeout — `timeout_ms` omitted → `calls[0].opts.timeoutMs === 300_000`.
- [x] **Test 8** wait explicit timeout — `timeout_ms: 1000` → `calls[0].opts.timeoutMs === 1000`.
- [x] **Test 9** wait timeout — stub returns `{ timedOut: true }` with `timeout_ms: 1234` → `isError: true`, message matches `/timed out/i` and contains `"1234"`.
- [x] **Test 10** wait non-zero exit — `code: 1`, stderr `"agent errored"` → `isError: true`, message contains `"agent errored"`.

### Implementation checklist
- [x] Add the `// ── mure tools ──` section to `pi-mure/index.ts` with `ExecFn`, `defaultExec`, `resolveMureBin`, `parseSpawnStdout`, `makeMureSpawnTool`, `makeMureWaitTool`, plus internal `okResult`/`errorResult` helpers.
- [x] Import `Type` and `type Static` from `"typebox"`; import `type ToolDefinition`, `type AgentToolResult` from `"@earendil-works/pi-coding-agent"`.
- [x] Do **not** wire the factories into the default `mureExtension(pi)` export yet — Phase 2 owns that.
- [x] Create `pi-mure/test/tools.test.ts` with tests 1–10 and `makeStubExec`.
- [x] Run `npm test` inside `pi-mure/`; iterate until green.

### Autonomous verification
Run from repo root:
```bash
cd pi-mure
npm test 2>&1 | tee /tmp/phase1.log
# Expected: existing suites (frame, gate, lifecycle, reconnect, cross) AND 10 new tools.test.ts cases all pass.
grep -E "# fail" /tmp/phase1.log | grep -v " 0$" && { echo FAIL: failures present; exit 1; } || echo OK
npx tsc --noEmit
diff <(node -e "console.log(Object.keys(require('./package.json').dependencies||{}).sort().join('\n'))") \
     <(git show HEAD:pi-mure/package.json | node -e "let s='';process.stdin.on('data',d=>s+=d).on('end',()=>console.log(Object.keys(JSON.parse(s).dependencies||{}).sort().join('\n')))")
# Expected: empty diff (no new runtime deps).
```
Phase passes when: all tests green, `tsc --noEmit` clean, deps diff empty.

---

## Phase 2: Gate-aware registration in `mureExtension` + registration tests
**Depends on:** Phase 1

Wire the factories from Phase 1 into the actual extension entrypoint behind the same env gate used by `start()`, and add the two registration tests (PRD §11 cases 11–12).

### Scope — additions to `pi-mure/index.ts`

- `export function mureEnabled(env: NodeJS.ProcessEnv): boolean` — returns `env.MURE_ENV === "1" && !!env.MURE_AGENT_ID && !!env.MURE_SOCKET` (PRD §5).
- Refactor the existing `start()` gate (currently `if (env.MURE_ENV !== "1" || !agentId || !socketPath) { ... }`) to call `mureEnabled(env)`. Behavior must be **byte-identical**; `gate.test.ts` must continue to pass unchanged.
- Update `mureExtension(pi)` body:
  - Existing event wiring stays.
  - After the existing `start({...})` call, add:
    ```ts
    if (mureEnabled(process.env)) {
      pi.registerTool(makeMureSpawnTool(defaultExec, process.env));
      pi.registerTool(makeMureWaitTool(defaultExec, process.env));
    }
    ```
  - When the gate is off, `start()` is already a no-op handle and no tools register — both PRD §5 requirements satisfied without further changes.

### Test additions in `pi-mure/test/tools.test.ts`

Add a `fakePi()` helper near the existing `makeStubExec`:
```ts
function fakePi() {
  const tools: ToolDefinition[] = [];
  const pi = {
    on: (_e: string, _h: any) => {},
    registerTool: (t: ToolDefinition) => { tools.push(t); },
  } as unknown as ExtensionAPI;
  return { pi, tools };
}
```
Both registration tests wrap env mutation in `try { ... } finally { restore }`:

- [x] **Test 11** gate off — delete `MURE_ENV`, `MURE_AGENT_ID`, `MURE_SOCKET` from `process.env`; call `mureExtension(pi)`; assert `tools.length === 0`.
- [x] **Test 12** gate on — set `process.env.MURE_ENV = "1"`, `MURE_AGENT_ID = "a"`, `MURE_SOCKET = "/tmp/x.sock"`; also inject `MURE_BIN` via env if needed; call `mureExtension(pi)`; assert `tools.map(t => t.name).sort()` equals `["mure_spawn", "mure_wait"]`. Then call the returned handle's `stop()` if any (Test 12 will cause `start()` to attempt a socket connection — to keep the test hermetic and prevent leaked file handles, set `MURE_SOCKET` to a temp path inside `os.tmpdir()` and tolerate the connection failing immediately; `start()` already swallows socket errors via its `"error"` handler and reschedules on `"close"`. To suppress the reconnect timer, capture the handle: refactor `mureExtension` to return its `Handle` so the test can call `.stop()`).

**Refactor required for Test 12 hermeticity:** change `mureExtension(pi)`'s declared return type from `void` to `Handle` and `return handle;` after the registration block. The pi loader accepts any return value (`ExtensionFactory` is `(pi) => void | Promise<void>` — returning a non-void value is harmless and ignored by the runtime). Justify this with a one-line comment.

### Implementation checklist
- [x] Add `mureEnabled(env)` export. Refactor `start()`'s gate to use it. Confirm `gate.test.ts` still passes unchanged.
- [x] Update `mureExtension(pi)` to conditionally register both tools after `start(...)`.
- [x] Change `mureExtension` to `return handle;` so tests can stop the socket layer.
- [x] Add `fakePi()` helper + Tests 11 and 12 to `pi-mure/test/tools.test.ts`.
- [x] Each registration test must restore prior `process.env` values in `finally`.
- [x] Confirm `pi-mure/package.json` `dependencies` still unchanged.

### Autonomous verification
```bash
cd pi-mure
npm test 2>&1 | tee /tmp/phase2.log
# Expected: 12 tools.test.ts cases + all pre-existing tests pass.
grep -E "# fail" /tmp/phase2.log | grep -v " 0$" && { echo FAIL; exit 1; } || echo OK
npx tsc --noEmit
# Sanity: confirm new test count grew by exactly 2 vs. Phase 1.
diff <(grep -c '^ok ' /tmp/phase2.log) <(grep -c '^ok ' /tmp/phase1.log) || echo "(expected difference of +2)"
```
Phase passes when: all 12 new tests + all pre-existing tests green; `tsc --noEmit` clean; no new deps.

---

## Phase 3: Manual acceptance — live mure pane sanity check
**Depends on:** Phase 2

Validates PRD §12 acceptance criterion #1 (tools visible and executable inside a real mure pane) with a one-time manual smoke. This phase produces no permanent test artifact (PRD §3 non-goal: no new socket/CLI work; reviewer flagged a permanent smoke script as scope creep). It produces a short manual-acceptance log committed under the spec directory.

### Scope
- Create `specs/003-pi-mure-tools/ACCEPTANCE.md` containing:
  - The exact shell steps to reproduce the smoke (build `mure`, start a mure session, launch a pi agent in a pane, observe both tools in `getAllTools()` output via pi's `/tools` slash command or equivalent, invoke `mure_spawn` with `role` set to any role already configured in the repo's `roles/` directory — discovered by listing that directory — then invoke `mure_wait` against the returned `agent_id`).
  - The actual captured output of each step from a run performed by the implementer.
  - Pass/fail verdict.
- Also a **gate-off** confirmation: launch `pi` outside a mure pane, run `/tools`, verify `mure_spawn` and `mure_wait` are absent.

### Implementation checklist
- [x] Inspect repo for the canonical mure-startup command (`Makefile` targets, `README.md`) and the list of pre-defined agent roles.
- [x] Execute the gate-on smoke: spawn → wait round-trip; capture pi's tool-call output.
- [x] Execute the gate-off smoke; capture `/tools` output.
- [x] Write `specs/003-pi-mure-tools/ACCEPTANCE.md` with both transcripts and a final `Acceptance: PASS` line.

### Autonomous verification
```bash
test -f specs/003-pi-mure-tools/ACCEPTANCE.md \
  && grep -q "Acceptance: PASS" specs/003-pi-mure-tools/ACCEPTANCE.md \
  && grep -q "mure_spawn" specs/003-pi-mure-tools/ACCEPTANCE.md \
  && grep -q "mure_wait" specs/003-pi-mure-tools/ACCEPTANCE.md \
  && echo PHASE3_OK || { echo PHASE3_FAIL; exit 1; }
# Also re-run unit tests to confirm no regressions:
(cd pi-mure && npm test) >/dev/null && echo TESTS_OK
```
Phase passes when: `ACCEPTANCE.md` exists with `Acceptance: PASS` and references both tool names; unit tests remain all-green.

---

## Cross-phase invariants (asserted in every phase's verification)
- `pi-mure/package.json` `dependencies` unchanged vs. `HEAD` at plan start.
- All pre-existing tests in `pi-mure/test/*.ts` pass unchanged (PRD §12.4).
- `tsc --noEmit` succeeds on `pi-mure/`.
