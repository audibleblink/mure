# PRD 003 — pi-mure Tools: `mure_spawn` & `mure_wait`

## 1. Summary

Expose two tools from the `pi-mure` extension that let a pi coding-agent
running inside a mure pane orchestrate sibling agents:

- `mure_spawn` — start a new mure-managed coding-agent in a new tmux pane.
- `mure_wait`  — block until a previously spawned agent emits its final
  result.

Both tools are thin wrappers around the existing `mure` CLI (`mure spawn`,
`mure wait`). They are registered **only** when the pi process is already
running inside a mure pane.

## 2. Goals

- Give in-pane pi agents a first-class way to fan out work and collect
  results without reimplementing the mure socket protocol.
- Zero new runtime dependencies; reuse the CLI as the source of truth.
- Stay invisible (unregistered) outside a mure environment.

## 3. Non-Goals

- No streaming of intermediate agent output (only final result text).
- No new socket frames, daemon endpoints, or CLI subcommands.
- No replacement of the existing `start()` lifecycle wiring.
- No tool for `mure kill`, `mure list`, or other CLI verbs (future work).
- No README docs update in this PRD (separate ask).

## 4. Tech Stack

| Layer | Choice |
|---|---|
| Tool registration | `ExtensionAPI` from `@earendil-works/pi-coding-agent` (already used) |
| Input schema | TypeBox (already transitively available via `pi-coding-agent`) |
| Process exec | `node:child_process` (`execFile`) — no new deps |
| Tests | Existing `pi-mure/test/` runner (`node --test` / whatever `frame.test.ts` uses), with an injected exec function |

## 5. Gating

Both tools register **iff all three are true**:

- `process.env.MURE_ENV === "1"`
- `process.env.MURE_AGENT_ID` is set
- `process.env.MURE_SOCKET` is set

This is the same gate as the existing frame emitter in `start()`. Outside
a mure pane the tools must not appear in the tool list at all (not just
return an error when called).

## 6. Binary Resolution

The child `mure` binary is resolved as:

1. `process.env.MURE_BIN` if set and non-empty.
2. Otherwise the literal string `"mure"` (trusts `PATH`).

Env passed to the child is `process.env` unchanged, so `MURE_SOCKET` and
friends propagate.

## 7. Tool: `mure_spawn`

### 7.1 Input schema (TypeBox)
```ts
Type.Object({
  role: Type.String({ minLength: 1 }),
  task: Type.Optional(Type.String()),
})
```

### 7.2 promptGuidelines
> Use `mure_spawn` to start a coding-agent subagent in a new tmux pane.
> Pair with `mure_wait` to collect its result.

### 7.3 Behavior
- Invokes: `mure spawn <role> [task]` via `execFile(bin, argv, { env })`.
  - `argv = ["spawn", role]` and, if `task` provided, `argv.push(task)`
    (passed as a single argv element — no shell interpolation).
- On exit code `0`, parse stdout.
- On non-zero exit, return a tool error containing stderr (trimmed) and
  the exit code.

### 7.4 stdout parsing
- `mure spawn` prints a single line of the form:
  ```
  <agent_id> <pane_id>
  ```
  Example: `agent-78f3cc08 %102`
- Parser:
  1. Take the last non-empty line of stdout (defensive against future
     banner lines).
  2. Split on whitespace; require exactly 2 tokens.
  3. Token 1 → `agent_id`. Token 2 → `pane_id` (must start with `%`).
- On parse failure, return a tool error including the raw stdout for
  debugging. Do not invent fields.

### 7.5 Return value
```ts
{ agent_id: string, pane_id: string }
```

## 8. Tool: `mure_wait`

### 8.1 Input schema (TypeBox)
```ts
Type.Object({
  agent_id: Type.String({ minLength: 1 }),
  timeout_ms: Type.Optional(Type.Integer({ minimum: 1 })),
})
```

### 8.2 promptGuidelines
> Use `mure_wait` to block until a previously spawned agent emits a
> result.

### 8.3 Behavior
- Default `timeout_ms` when omitted: **300_000** (5 minutes).
- Invokes: `mure wait <agent_id>` via `execFile`.
- On exit code `0`, return stdout (final result text) — trimmed of a
  single trailing newline; otherwise verbatim.
- On non-zero exit, return a tool error containing stderr (trimmed) and
  the exit code. This covers the `errored`-with-no-result case that the
  CLI signals via non-zero exit.
- On timeout:
  - Kill the child (`SIGTERM`, then `SIGKILL` after 2s grace).
  - Return a tool error: `"mure_wait timed out after <N>ms"`.
  - The daemon-side agent continues running; the caller may re-issue
    `mure_wait` with a longer timeout.

### 8.4 Return value
- Success: a single `string` (the agent's final result text).

## 9. Implementation Notes

- All new code lives in `pi-mure/index.ts` next to `mureExtension()`.
- Factor the exec call behind an injectable function for testability,
  mirroring how `start()` injects `connect`:
  ```ts
  export type ExecFn = (
    bin: string,
    argv: string[],
    opts: { env: NodeJS.ProcessEnv; timeoutMs?: number },
  ) => Promise<{ code: number; stdout: string; stderr: string; timedOut: boolean }>;
  ```
  Default implementation wraps `child_process.execFile` and tracks
  timeout via `setTimeout` + `child.kill`.
- Tool registration happens inside `mureExtension(pi)` only if the same
  gate from `start()` is satisfied. A small helper `mureEnabled(env)`
  centralizes the predicate (also usable by `start()`).
- Export the tool factories (e.g. `makeMureSpawnTool`, `makeMureWaitTool`)
  and the `ExecFn` type so tests can drive them directly without
  spinning up the full extension.

## 10. Files Touched

- `pi-mure/index.ts` — add tool factories, exec abstraction, register
  inside `mureExtension`.
- `pi-mure/test/tools.test.ts` — new, see §11.

No changes to: daemon, sock protocol, CLI, sidebar, tmux plugin, or any
other `pi-mure/test/*.ts`.

## 11. Tests (`pi-mure/test/tools.test.ts`)

All tests use a stub `ExecFn` that records `(bin, argv, opts)` and
returns canned `{ code, stdout, stderr, timedOut }`.

### Required cases
1. **spawn happy path (no task)** — argv is exactly
   `["spawn", "planner"]`; stdout `"agent-abc %12\n"`; result equals
   `{ agent_id: "agent-abc", pane_id: "%12" }`.
2. **spawn happy path (with task)** — argv is exactly
   `["spawn", "builder", "fix the flaky test"]` (task passed as one
   argv element, not concatenated/quoted).
3. **spawn honors MURE_BIN** — when `env.MURE_BIN = "/opt/mure/bin/mure"`,
   `bin` arg to ExecFn is that path; otherwise `"mure"`.
4. **spawn parse failure** — stdout `"weird banner\n"` → tool error;
   error message includes the raw stdout.
5. **spawn non-zero exit** — code `2`, stderr `"role unknown\n"` →
   tool error containing stderr and exit code.
6. **wait happy path** — argv is `["wait", "agent-abc"]`; stdout
   `"final answer text\n"`; result equals `"final answer text"`.
7. **wait default timeout** — when `timeout_ms` omitted, ExecFn is
   called with `opts.timeoutMs === 300_000`.
8. **wait explicit timeout** — `timeout_ms: 1000` → `opts.timeoutMs === 1000`.
9. **wait timeout** — ExecFn returns `{ timedOut: true }` → tool error
   matching `/timed out/i` and mentions the configured ms.
10. **wait non-zero exit (errored agent)** — code `1`, stderr
    `"agent errored"` → tool error containing stderr.
11. **gate off** — with `MURE_ENV` unset, calling the public registrar
    against a fake `ExtensionAPI` records **zero** tool registrations.
12. **gate on** — with `MURE_ENV=1`, `MURE_AGENT_ID`, `MURE_SOCKET` all
    set, exactly two tools are registered with names `mure_spawn` and
    `mure_wait`.

## 12. Acceptance Criteria

1. Inside a mure pane (gate on), `mure_spawn` and `mure_wait` are
   visible to pi and executable end-to-end against a real `mure`
   binary.
2. Outside a mure pane (any of the three gate vars missing/empty),
   neither tool is registered.
3. All 12 tests in §11 pass.
4. All pre-existing `pi-mure/test/*.ts` tests pass unchanged.
5. No new entries in `pi-mure/package.json` `dependencies`.

## 13. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| `mure spawn` stdout format changes | Parser takes the last non-empty line + validates pane_id starts with `%`; failure returns a clear tool error rather than garbage |
| Long-running waits block the pi agent loop | Default 5-minute timeout; caller can pass any positive `timeout_ms`; timeout returns a tool error so the agent can decide to retry |
| Child `mure` inherits a stale or wrong `MURE_SOCKET` | Env is inherited verbatim from the pi process, which itself was launched by mure — same socket guaranteed by construction |
| Tests flaky due to real spawn | Tests never invoke a real binary; ExecFn is injected |

## 14. Out-of-Scope Follow-Ups

- `mure_list` / `mure_kill` tools.
- Streaming intermediate output / status events to the caller.
- README "Tools" section in `pi-mure/README.md`.
- Surfacing `pane_id` to the agent via a structured second return
  channel (currently returned as a plain field on `mure_spawn`).
