import test from "node:test";
import assert from "node:assert/strict";
import os from "node:os";
import path from "node:path";
import mureExtension, {
  type ExecFn,
  makeMureSpawnTool,
  makeMureWaitTool,
} from "../index.ts";
import type { ExtensionAPI, ToolDefinition } from "@earendil-works/pi-coding-agent";

function fakePi() {
  const tools: ToolDefinition<any>[] = [];
  const pi = {
    on: (_e: string, _h: any) => {},
    registerTool: (t: ToolDefinition<any>) => { tools.push(t); },
  } as unknown as ExtensionAPI;
  return { pi, tools };
}

function withEnv(patch: Record<string, string | undefined>, fn: () => void) {
  const keys = Object.keys(patch);
  const prev: Record<string, string | undefined> = {};
  for (const k of keys) prev[k] = process.env[k];
  for (const k of keys) {
    if (patch[k] === undefined) delete process.env[k];
    else process.env[k] = patch[k];
  }
  try { fn(); } finally {
    for (const k of keys) {
      if (prev[k] === undefined) delete process.env[k];
      else process.env[k] = prev[k];
    }
  }
}

function makeStubExec(
  canned: { code?: number; stdout?: string; stderr?: string; timedOut?: boolean } = {},
) {
  const calls: Array<{
    bin: string;
    argv: string[];
    opts: { env: NodeJS.ProcessEnv; timeoutMs?: number };
  }> = [];
  const exec: ExecFn = async (bin, argv, opts) => {
    calls.push({ bin, argv, opts });
    return { code: 0, stdout: "", stderr: "", timedOut: false, ...canned };
  };
  return { exec, calls };
}

const ctx = {} as any;

test("Test 1: spawn happy path without task", async () => {
  const { exec, calls } = makeStubExec({ stdout: "agent-abc %12\n" });
  const tool = makeMureSpawnTool(exec, {} as NodeJS.ProcessEnv);
  const r = await tool.execute!("id", { role: "planner" }, undefined as any, undefined as any, ctx);
  assert.deepEqual(calls[0].argv, ["spawn", "planner"]);
  assert.equal(r.isError, undefined);
  assert.deepEqual(JSON.parse(r.content[0].text), { agent_id: "agent-abc", pane_id: "%12" });
});

test("Test 2: spawn happy path with task", async () => {
  const { exec, calls } = makeStubExec({ stdout: "agent-xyz %3\n" });
  const tool = makeMureSpawnTool(exec, {} as NodeJS.ProcessEnv);
  await tool.execute!("id", { role: "builder", task: "fix the flaky test" }, undefined as any, undefined as any, ctx);
  assert.deepEqual(calls[0].argv, ["spawn", "builder", "fix the flaky test"]);
});

test("Test 3: spawn honors MURE_BIN", async () => {
  const a = makeStubExec({ stdout: "a %1\n" });
  await makeMureSpawnTool(a.exec, { MURE_BIN: "/opt/mure/bin/mure" } as any).execute!(
    "id", { role: "r" }, undefined as any, undefined as any, ctx,
  );
  assert.equal(a.calls[0].bin, "/opt/mure/bin/mure");

  const b = makeStubExec({ stdout: "a %1\n" });
  await makeMureSpawnTool(b.exec, {} as NodeJS.ProcessEnv).execute!(
    "id", { role: "r" }, undefined as any, undefined as any, ctx,
  );
  assert.equal(b.calls[0].bin, "mure");
});

test("Test 4: spawn parse failure", async () => {
  const { exec } = makeStubExec({ stdout: "weird banner\n" });
  const r = await makeMureSpawnTool(exec, {} as NodeJS.ProcessEnv).execute!(
    "id", { role: "r" }, undefined as any, undefined as any, ctx,
  );
  assert.equal(r.isError, true);
  assert.match(r.content[0].text, /weird banner/);
});

test("Test 5: spawn non-zero exit", async () => {
  const { exec } = makeStubExec({ code: 2, stderr: "role unknown\n" });
  const r = await makeMureSpawnTool(exec, {} as NodeJS.ProcessEnv).execute!(
    "id", { role: "r" }, undefined as any, undefined as any, ctx,
  );
  assert.equal(r.isError, true);
  assert.match(r.content[0].text, /role unknown/);
  assert.match(r.content[0].text, /2/);
});

test("Test 6: wait happy path", async () => {
  const { exec, calls } = makeStubExec({ stdout: "final answer text\n" });
  const r = await makeMureWaitTool(exec, {} as NodeJS.ProcessEnv).execute!(
    "id", { agent_id: "agent-abc" }, undefined as any, undefined as any, ctx,
  );
  assert.deepEqual(calls[0].argv, ["wait", "agent-abc"]);
  assert.equal(r.isError, undefined);
  assert.equal(r.content[0].text, "final answer text");
});

test("Test 7: wait default timeout", async () => {
  const { exec, calls } = makeStubExec({ stdout: "x\n" });
  await makeMureWaitTool(exec, {} as NodeJS.ProcessEnv).execute!(
    "id", { agent_id: "a" }, undefined as any, undefined as any, ctx,
  );
  assert.equal(calls[0].opts.timeoutMs, 300_000);
});

test("Test 8: wait explicit timeout", async () => {
  const { exec, calls } = makeStubExec({ stdout: "x\n" });
  await makeMureWaitTool(exec, {} as NodeJS.ProcessEnv).execute!(
    "id", { agent_id: "a", timeout_ms: 1000 }, undefined as any, undefined as any, ctx,
  );
  assert.equal(calls[0].opts.timeoutMs, 1000);
});

test("Test 9: wait timeout", async () => {
  const { exec } = makeStubExec({ timedOut: true });
  const r = await makeMureWaitTool(exec, {} as NodeJS.ProcessEnv).execute!(
    "id", { agent_id: "a", timeout_ms: 1234 }, undefined as any, undefined as any, ctx,
  );
  assert.equal(r.isError, true);
  assert.match(r.content[0].text, /timed out/i);
  assert.match(r.content[0].text, /1234/);
});

test("Test 10: wait non-zero exit", async () => {
  const { exec } = makeStubExec({ code: 1, stderr: "agent errored" });
  const r = await makeMureWaitTool(exec, {} as NodeJS.ProcessEnv).execute!(
    "id", { agent_id: "a" }, undefined as any, undefined as any, ctx,
  );
  assert.equal(r.isError, true);
  assert.match(r.content[0].text, /agent errored/);
});

test("Test 11: mureExtension registers no tools when gate is off", () => {
  withEnv(
    { MURE_ENV: undefined, MURE_AGENT_ID: undefined, MURE_SOCKET: undefined },
    () => {
      const { pi, tools } = fakePi();
      const handle = mureExtension(pi);
      handle.stop();
      assert.equal(tools.length, 0);
    },
  );
});

test("Test 12: mureExtension registers both tools when gate is on", () => {
  const sock = path.join(os.tmpdir(), `mure-test-${process.pid}.sock`);
  withEnv(
    { MURE_ENV: "1", MURE_AGENT_ID: "a", MURE_SOCKET: sock },
    () => {
      const { pi, tools } = fakePi();
      const handle = mureExtension(pi);
      handle.stop();
      assert.deepEqual(tools.map((t) => t.name).sort(), ["mure_spawn", "mure_wait"]);
    },
  );
});
