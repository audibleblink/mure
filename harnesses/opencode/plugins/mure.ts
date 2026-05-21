// mure.ts — opencode plugin that bridges tool/session lifecycle events to
// the mure daemon. Installed by `mure integration install opencode` into
// ~/.config/opencode/plugins/mure.ts.
//
// Wire protocol: one transient ("oneshot") agent connection per emit. We
// send a hello frame then one status/result frame, then close. This avoids
// holding the agent slot — the daemon must not Remove() on oneshot close.
//
// Env contract (set by `mure spawn` on the spawned pane):
//   MURE_SOCKET     unix socket path
//   MURE_AGENT_ID   agent identifier the daemon expects
//   MURE_PANE_ID    tmux pane id (optional; falls back to TMUX_PANE)

import type { Plugin } from "@opencode-ai/plugin"
import { connect } from "node:net"

const SOCK = process.env.MURE_SOCKET
const AGENT = process.env.MURE_AGENT_ID
const PANE = process.env.MURE_PANE_ID ?? process.env.TMUX_PANE ?? ""

function emit(frame: Record<string, unknown>): Promise<void> {
  if (!SOCK || !AGENT) return Promise.resolve()
  return new Promise((resolve) => {
    const c = connect(SOCK!, () => {
      const hello = {
        v: 1,
        event: "hello",
        role: "agent",
        agent_id: AGENT,
        pane_id: PANE,
        oneshot: true,
        ts: Date.now(),
      }
      c.write(JSON.stringify(hello) + "\n")
      c.write(JSON.stringify({ v: 1, agent_id: AGENT, ts: Date.now(), ...frame }) + "\n")
      c.end()
    })
    c.on("close", () => resolve())
    c.on("error", () => resolve()) // best-effort; never break the agent
  })
}

export const MurePlugin: Plugin = async ({ client }) => ({
  "tool.execute.before": async (input: { tool: string }) => {
    await emit({ event: "status", status: "working", tool: input.tool })
  },
  "tool.execute.after": async () => {
    await emit({ event: "status", status: "idle" })
  },
  event: async ({ event }: { event: any }) => {
    if (event?.type !== "session.idle") return
    const sid = event?.properties?.sessionID
    if (!sid) return
    // Pull the last assistant message text and ship it as the result.
    const res = await client.session.messages({ path: { id: sid } }).catch(() => null)
    const list: any[] = res?.data ?? []
    const last = [...list].reverse().find((m) => m?.role === "assistant")
    const text: string =
      last?.parts
        ?.filter((p: any) => p?.type === "text")
        .map((p: any) => p.text)
        .join("\n") ?? ""
    if (text) await emit({ event: "result", text })
  },
})
