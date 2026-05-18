import { test } from "node:test";
import assert from "node:assert/strict";
import { start, decodeFrame } from "../index.ts";
import { makeServer, FakeBus } from "./_server.ts";

test("lifecycle events produce expected frame sequence", async () => {
  const server = await makeServer();
  const bus = new FakeBus();
  let t = 1_000_000;
  const h = start({
    env: { MURE_ENV: "1", MURE_AGENT_ID: "agent-1", MURE_SOCKET: server.path, TMUX_PANE: "%9" },
    pid: 4242,
    piVersion: "0.50.3",
    bus,
    now: () => ++t,
  });

  // Wait for hello to land.
  await server.waitForFrame(0, 1);

  bus.emit({ type: "session_start" });
  bus.emit({ type: "agent_start", task: "refactor" });
  bus.emit({ type: "tool_execution_start", tool: "bash" });
  bus.emit({ type: "tool_blocked" });
  bus.emit({ type: "agent_end" });
  bus.emit({ type: "session_shutdown" });

  await server.conns[0].done;
  h.stop();

  const frames = server.conns[0].frames.map(decodeFrame);
  // hello, then 5 statuses, then bye.
  assert.equal(frames.length, 7);
  assert.equal(frames[0].event, "hello");
  assert.equal((frames[0] as any).agent_id, "agent-1");
  assert.equal((frames[0] as any).pane_id, "%9");
  assert.equal((frames[0] as any).pid, 4242);
  assert.equal((frames[0] as any).pi_version, "0.50.3");

  assert.deepEqual(
    frames.slice(1, 6).map((f: any) => ({ event: f.event, status: f.status, task: f.task, tool: f.tool })),
    [
      { event: "status", status: "idle", task: undefined, tool: undefined },
      { event: "status", status: "working", task: "refactor", tool: undefined },
      { event: "status", status: "working", task: undefined, tool: "bash" },
      { event: "status", status: "blocked", task: undefined, tool: undefined },
      { event: "status", status: "idle", task: undefined, tool: undefined },
    ],
  );

  assert.equal(frames[6].event, "bye");
  assert.equal((frames[6] as any).agent_id, "agent-1");

  await server.close();
});
