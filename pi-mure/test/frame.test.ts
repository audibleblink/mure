// Golden frame encode/decode against PRD §12 examples.
// Must round-trip with the exact field order documented there.
import { test } from "node:test";
import assert from "node:assert/strict";
import { encodeFrame, decodeFrame, type Frame } from "../index.ts";

const goldenAgent = [
  '{"v":1,"event":"hello","role":"agent","agent_id":"agent-3","pane_id":"%41","pid":12345,"pi_version":"0.50.3","ts":1731890000000}',
  '{"v":1,"event":"status","agent_id":"agent-3","status":"working","task":"refactor auth","tool":"bash","ts":1731890001234}',
  '{"v":1,"event":"bye","agent_id":"agent-3","ts":1731890003000}',
];

test("encode hello (agent) matches PRD byte-for-byte", () => {
  const f: Frame = {
    v: 1,
    event: "hello",
    role: "agent",
    agent_id: "agent-3",
    pane_id: "%41",
    pid: 12345,
    pi_version: "0.50.3",
    ts: 1731890000000,
  };
  assert.equal(encodeFrame(f), goldenAgent[0] + "\n");
});

test("encode status matches PRD byte-for-byte", () => {
  const f: Frame = {
    v: 1,
    event: "status",
    agent_id: "agent-3",
    status: "working",
    task: "refactor auth",
    tool: "bash",
    ts: 1731890001234,
  };
  assert.equal(encodeFrame(f), goldenAgent[1] + "\n");
});

test("encode bye matches PRD byte-for-byte", () => {
  const f: Frame = { v: 1, event: "bye", agent_id: "agent-3", ts: 1731890003000 };
  assert.equal(encodeFrame(f), goldenAgent[2] + "\n");
});

test("decode round-trips PRD golden frames", () => {
  for (const g of goldenAgent) {
    const f = decodeFrame(g + "\n");
    assert.equal(encodeFrame(f), g + "\n");
  }
});
