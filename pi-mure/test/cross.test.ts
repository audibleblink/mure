// Cross-protocol golden: every fixture frame must round-trip through
// encodeFrame and produce a JSON-equivalent output. Asserts identical
// byte sequence after key-order normalization (which our encoder does
// via the typed struct shape).
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import { encodeFrame, decodeFrame } from "../index.ts";

const here = dirname(fileURLToPath(import.meta.url));
const raw = readFileSync(join(here, "fixtures", "frames.json"), "utf8");
const fx = JSON.parse(raw) as Record<string, unknown>;

test("every fixture decodes and re-encodes byte-identically (TS side)", () => {
  for (const [name, frame] of Object.entries(fx)) {
    const line = JSON.stringify(frame);
    const decoded = decodeFrame(line + "\n");
    const re = encodeFrame(decoded).slice(0, -1);
    assert.equal(re, line, `mismatch for ${name}`);
  }
});
