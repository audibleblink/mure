import { test } from "node:test";
import assert from "node:assert/strict";
import { start, decodeFrame } from "../index.ts";
import { makeServer, FakeBus } from "./_server.ts";

test("reconnects after server drop and coalesces buffered status frames", async () => {
  const server = await makeServer();
  const bus = new FakeBus();
  let t = 1_000;
  const h = start({
    env: { MURE_ENV: "1", MURE_AGENT_ID: "agent-1", MURE_SOCKET: server.path },
    pid: 1,
    piVersion: "x",
    bus,
    now: () => ++t,
    initialBackoffMs: 5,
    maxBackoffMs: 50,
    random: () => 0, // delay = 0
  });

  // Wait for initial hello.
  await server.waitForFrame(0, 1);

  // Drop the connection from the server side.
  server.drop(0);

  // While disconnected, emit several lifecycle events; status frames must coalesce.
  bus.emit({ type: "agent_start", task: "a" });
  bus.emit({ type: "tool_execution_start", tool: "bash" });
  bus.emit({ type: "tool_blocked" });
  bus.emit({ type: "agent_end" });

  // Wait for the second connection.
  const c2 = await server.waitForConn(1);
  // Expect: hello + exactly one coalesced status (the latest "idle").
  // The client may also resend currentStatus first if a status had been
  // sent pre-disconnect. In this test, no status was sent before the drop,
  // so we expect exactly hello + one buffered status.
  await server.waitForFrame(1, 2);
  // Give a tick for any further straggling writes.
  await new Promise((r) => setTimeout(r, 30));

  const frames = c2.frames.map(decodeFrame);
  assert.equal(frames.length, 2, `got frames: ${JSON.stringify(frames)}`);
  assert.equal(frames[0].event, "hello");
  assert.equal(frames[1].event, "status");
  assert.equal((frames[1] as any).status, "idle"); // last buffered status wins

  h.stop();
  await server.close();
});

test("re-sends current status on reconnect when one was already delivered", async () => {
  const server = await makeServer();
  const bus = new FakeBus();
  const h = start({
    env: { MURE_ENV: "1", MURE_AGENT_ID: "agent-1", MURE_SOCKET: server.path },
    bus,
    initialBackoffMs: 5,
    maxBackoffMs: 50,
    random: () => 0,
  });

  await server.waitForFrame(0, 1);
  bus.emit({ type: "agent_start", task: "do-thing" });
  await server.waitForFrame(0, 2);
  server.drop(0);

  await server.waitForConn(1);
  await server.waitForFrame(1, 2);
  await new Promise((r) => setTimeout(r, 30));

  const frames = server.conns[1].frames.map(decodeFrame);
  assert.equal(frames[0].event, "hello");
  assert.equal(frames[1].event, "status");
  assert.equal((frames[1] as any).status, "working");
  assert.equal((frames[1] as any).task, "do-thing");

  h.stop();
  await server.close();
});
