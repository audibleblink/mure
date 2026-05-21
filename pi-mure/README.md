# pi-mure

[pi](https://github.com/earendil-works/pi-coding-agent) extension that streams
coding-agent lifecycle events into the [`mure`](../README.md) daemon.

The extension is a thin, dependency-free shim. It subscribes to `pi` lifecycle
hooks, translates them into `mure` protocol frames, and writes NDJSON over the
per-session Unix socket the daemon owns.

## Activation

The extension is **inert** unless all three are set in the environment:

| Var | Set by | Purpose |
|---|---|---|
| `MURE_ENV=1` | `mure spawn` | Opt-in flag. |
| `MURE_AGENT_ID` | `mure spawn` | Stable agent identity. |
| `MURE_SOCKET` | `mure spawn` | Daemon socket path. |

In practice you never set these by hand — `mure spawn` injects them into the
pane's env. Outside a `mure`-managed pane the extension is a no-op.

Optional inputs: `TMUX_PANE`, `MURE_ROLE`, `MURE_TASK`, `PI_VERSION`, `MURE_BIN`.

## Install

From a checkout of this repo:

```sh
cd pi-mure
npm install
pi extension install .
```

Or let mure manage it:

```sh
mure integration install pi
```

## How it works

`mureExtension(pi)` (the default export) wires `pi` events to a small state
machine:

| pi event | Frame emitted |
|---|---|
| `session_start` | `status: idle` |
| `agent_start` | `status: working` (with `task` from `$MURE_TASK`) |
| `tool_execution_start` | `status: working` (with `tool` name) |
| `tool_blocked` | `status: blocked` |
| `agent_end` | `result` (final assistant text) then `status: idle` |
| `session_shutdown` | `bye`, then disconnect |

On connect the extension always sends a `hello` frame first, then re-emits the
last known `status` so the daemon can recover state across reconnects.

## Tools

When activated, the extension registers two `pi` tools so a coding agent can
fan out work to subagents. Both shell out to the `mure` binary (override with
`MURE_BIN`) and are only available in mure-managed panes — outside that
environment the extension is inert and registers nothing.

### `mure_spawn`

Start a mure-managed subagent in a new tmux pane.

| Param | Type | Required | Notes |
|---|---|---|---|
| `role` | string (min length 1) | yes | Subagent role label. |
| `task` | string | no | Initial prompt; passed as `$MURE_TASK` and as a positional arg to the agent. |

Returns a JSON string `{"agent_id": "agent-…", "pane_id": "%N"}` parsed from
the last line of `mure spawn` stdout.

### `mure_wait`

Block until a previously spawned agent emits its final result.

| Param | Type | Required | Notes |
|---|---|---|---|
| `agent_id` | string (min length 1) | yes | ID returned by `mure_spawn`. |
| `timeout_ms` | integer ≥ 1 | no | Defaults to `300_000` (5 min). On timeout the child is `SIGTERM`'d (then `SIGKILL` after 2 s) and the tool returns an error result. |

Returns the agent's result text (trailing newline stripped).

### Reliability

- **Reconnect:** full-jitter exponential backoff, `250 ms` → `30 s` cap.
- **Buffering:** frames queued while disconnected. `status` frames are
  coalesced — only the latest is kept.
- **Ordering:** `hello` is always first on each connection; the queued tail
  follows.
- **No throws:** socket errors are swallowed; `close` drives reconnect.

### Protocol

NDJSON, one frame per line. Schema version `v: 1`. Frame types:

```ts
HelloFrame  { event: "hello",  role, agent_id, pane_id?, pid, pi_version, ts }
StatusFrame { event: "status", agent_id, status, task?, tool?, ts }
ResultFrame { event: "result", agent_id, text, ts }
ByeFrame    { event: "bye",    agent_id, ts }
```

`status` is one of `idle | working | blocked | disconnected | errored`.

See `index.ts` for the authoritative TypeScript types.

## Architecture

```
pi runtime ── lifecycle events ──▶ mureExtension()
                                          │
                                          ▼
                                   start() state machine
                                   ├─ hello on (re)connect
                                   ├─ status coalescing queue
                                   └─ backoff reconnect
                                          │
                                          ▼ NDJSON
                                   $MURE_SOCKET (Unix)
                                          │
                                          ▼
                                   mure daemon
```

`start()` is exported separately and accepts a `StartOptions` bag for
dependency injection (custom `connect`, `now`, `bus`, timers, RNG) — that is
the seam the tests drive.

## Development

```sh
npm install
npm test     # node --test against test/*.test.ts (uses tsx loader)
```

The source-of-truth lives here; `make sync-piext` (at the repo root) mirrors
this directory into `internal/piext/assets/` so the Go daemon can embed it.
CI fails if the mirror drifts.
