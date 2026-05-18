import { test } from "node:test";
import assert from "node:assert/strict";
import { start } from "../index.ts";

test("no-op without MURE_ENV", () => {
  let called = false;
  const h = start({
    env: { MURE_AGENT_ID: "agent-1", MURE_SOCKET: "/tmp/x.sock" },
    connect: () => {
      called = true;
      throw new Error("should not connect");
    },
  });
  h.stop();
  assert.equal(called, false);
});

test("no-op without MURE_AGENT_ID", () => {
  let called = false;
  const h = start({
    env: { MURE_ENV: "1", MURE_SOCKET: "/tmp/x.sock" },
    connect: () => {
      called = true;
      throw new Error("should not connect");
    },
  });
  h.stop();
  assert.equal(called, false);
});

test("no-op without MURE_SOCKET", () => {
  let called = false;
  const h = start({
    env: { MURE_ENV: "1", MURE_AGENT_ID: "agent-1" },
    connect: () => {
      called = true;
      throw new Error("should not connect");
    },
  });
  h.stop();
  assert.equal(called, false);
});
