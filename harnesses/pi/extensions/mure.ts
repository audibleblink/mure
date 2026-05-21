// mure.ts — pi extension that bridges tool/agent lifecycle events to the
// mure daemon. Installed by `mure integration install pi` into
// $PI_CODING_AGENT_DIR/extensions/mure.ts (default ~/.config/pi/agent).
//
// Wire protocol: one transient ("oneshot") agent connection per emit. We
// send a hello frame then one status/result frame, then close. This avoids
// holding the agent slot — the daemon must not Remove() on oneshot close.
//
// Env contract (set by `mure spawn` on the spawned pane):
//   MURE_SOCKET     unix socket path
//   MURE_AGENT_ID   agent identifier the daemon expects
//   MURE_PANE_ID    tmux pane id (optional; falls back to TMUX_PANE)
//   MURE_ROLE       agent role/label (optional; surfaces in the sidebar)

import type { ExtensionAPI } from "@earendil-works/pi-coding-agent"
import { connect } from "node:net"
import { appendFileSync } from "node:fs"

const SOCK = process.env.MURE_SOCKET
const AGENT = process.env.MURE_AGENT_ID
const PANE = process.env.MURE_PANE_ID ?? process.env.TMUX_PANE ?? ""
const ROLE = process.env.MURE_ROLE ?? ""

// Debug: set MURE_DEBUG_LOG=/tmp/mure-pi-debug.log to trace extension activity.
const DEBUG = process.env.MURE_DEBUG_LOG
function dbg(line: string) {
  if (!DEBUG) return
  try {
    appendFileSync(DEBUG, `${new Date().toISOString()} ${line}\n`)
  } catch {}
}

function emit(frame: Record<string, unknown>): Promise<void> {
  if (!SOCK || !AGENT) return Promise.resolve()
  return new Promise((resolve) => {
    const c = connect(SOCK!, () => {
      const hello: Record<string, unknown> = {
        v: 1,
        event: "hello",
        role: "agent",
        agent_id: AGENT,
        pane_id: PANE,
        oneshot: true,
        ts: Date.now(),
      }
      if (ROLE) hello.agent_role = ROLE
      c.write(JSON.stringify(hello) + "\n")
      c.write(JSON.stringify({ v: 1, agent_id: AGENT, ts: Date.now(), ...frame }) + "\n")
      c.end()
    })
    c.on("close", () => resolve())
    c.on("error", () => resolve()) // best-effort; never break the agent
  })
}

export default function (pi: ExtensionAPI) {
  dbg(`load sock=${SOCK ?? ""} agent=${AGENT ?? ""} pane=${PANE}`)

  // Map pi events to mure status frames using the same semantics as the
  // Claude hooks plugin:
  //   before_agent_start  -> working  (inference begins)
  //   tool_execution_end  -> working  (tool done, back to inference)
  //   agent_end           -> result + idle (turn over, awaiting user)
  // Pi has no permission-prompt concept, so there's no `blocked` analog.

  pi.on("before_agent_start", async () => {
    dbg(`before_agent_start`)
    await emit({ event: "status", status: "working" })
  })
  pi.on("tool_execution_end", async () => {
    dbg(`tool_execution_end`)
    await emit({ event: "status", status: "working" })
  })
  pi.on("agent_end", async (event) => {
    dbg(`agent_end`)
    // Pull the last assistant message text and ship it as the result.
    const msgs: any[] = (event as any)?.messages ?? []
    const last = [...msgs].reverse().find((m) => m?.role === "assistant")
    const parts: any[] = last?.content ?? []
    const text = parts
      .filter((p) => p?.type === "text")
      .map((p) => p.text)
      .join("\n")
    if (text) await emit({ event: "result", text })
    await emit({ event: "status", status: "idle" })
  })
}
