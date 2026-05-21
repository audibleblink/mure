// mure pi extension (PRD §10).
// Gated on MURE_ENV=1 + MURE_AGENT_ID. Speaks NDJSON over MURE_SOCKET (PRD §12).

import net from "node:net";
import { spawn } from "node:child_process";
import { Type } from "typebox";
import type { ExtensionAPI, ToolDefinition, AgentToolResult } from "@earendil-works/pi-coding-agent";

export const PROTOCOL_VERSION = 1;

export type Frame =
  | HelloFrame
  | StatusFrame
  | ResultFrame
  | ByeFrame;

export interface HelloFrame {
  v: 1;
  event: "hello";
  role: "agent";
  agent_id: string;
  pane_id?: string;
  agent_role?: string;
  pid: number;
  pi_version: string;
  ts: number;
}

export interface StatusFrame {
  v: 1;
  event: "status";
  agent_id: string;
  status: "idle" | "working" | "blocked";
  task?: string;
  tool?: string;
  ts: number;
}

export interface ResultFrame {
  v: 1;
  event: "result";
  agent_id: string;
  text: string;
  ts: number;
}

export interface ByeFrame {
  v: 1;
  event: "bye";
  agent_id: string;
  ts: number;
}

// Tool names whose `tool_execution_start` should report status=blocked
// instead of status=working. These are tools that suspend pi awaiting user
// input (see pi's ctx.ui.custom). Extend as new interactive tools appear.
const BLOCKING_TOOLS = new Set<string>(["ask_user_question"]);

export type LifecycleEvent =
  | { type: "session_start" }
  | { type: "agent_start"; task?: string }
  | { type: "tool_execution_start"; tool?: string }
  | { type: "tool_execution_end"; tool?: string }
  | { type: "tool_blocked"; tool?: string }
  | { type: "agent_end"; result?: string }
  | { type: "session_shutdown" };

export interface LifecycleBus {
  on(handler: (e: LifecycleEvent) => void): void;
}

export interface ConnectFn {
  (path: string): net.Socket;
}

export interface StartOptions {
  env?: NodeJS.ProcessEnv;
  pid?: number;
  piVersion?: string;
  bus?: LifecycleBus;
  now?: () => number;
  connect?: ConnectFn;
  initialBackoffMs?: number;
  maxBackoffMs?: number;
  random?: () => number;
  setTimeoutFn?: (fn: () => void, ms: number) => unknown;
  clearTimeoutFn?: (h: unknown) => void;
}

export interface Handle {
  stop(): void;
  // Test introspection:
  _bufferSize(): number;
  _connected(): boolean;
}

export function encodeFrame(f: Frame): string {
  return JSON.stringify(f) + "\n";
}

export function decodeFrame(line: string): Frame {
  const t = line.endsWith("\n") ? line.slice(0, -1) : line;
  return JSON.parse(t) as Frame;
}

export function mureEnabled(env: NodeJS.ProcessEnv): boolean {
  return env.MURE_ENV === "1" && !!env.MURE_AGENT_ID && !!env.MURE_SOCKET;
}

export function start(opts: StartOptions = {}): Handle {
  const env = opts.env ?? process.env;
  const agentId = env.MURE_AGENT_ID;
  const socketPath = env.MURE_SOCKET;
  if (!mureEnabled(env)) {
    return { stop() {}, _bufferSize: () => 0, _connected: () => false };
  }

  const pid = opts.pid ?? process.pid;
  const piVersion = opts.piVersion ?? "unknown";
  const now = opts.now ?? Date.now;
  const connectFn: ConnectFn = opts.connect ?? ((p) => net.createConnection(p));
  const initialBackoff = opts.initialBackoffMs ?? 250;
  const maxBackoff = opts.maxBackoffMs ?? 30_000;
  const random = opts.random ?? Math.random;
  const setT = opts.setTimeoutFn ?? ((fn, ms) => setTimeout(fn, ms));
  const clearT = opts.clearTimeoutFn ?? ((h) => clearTimeout(h as ReturnType<typeof setTimeout>));

  let sock: net.Socket | null = null;
  let connected = false;
  let stopped = false;
  let reconnectAttempt = 0;
  let reconnectTimer: unknown = null;
  let currentStatus: StatusFrame | null = null;
  // Outbound queue used while disconnected.
  const queue: Frame[] = [];

  const helloFrame = (): HelloFrame => ({
    v: 1,
    event: "hello",
    role: "agent",
    agent_id: agentId,
    pane_id: env.TMUX_PANE,
    agent_role: env.MURE_ROLE,
    pid,
    pi_version: piVersion,
    ts: now(),
  });

  function enqueue(f: Frame) {
    if (f.event === "status") {
      // Coalesce: replace the last buffered status, if any.
      for (let i = queue.length - 1; i >= 0; i--) {
        if (queue[i].event === "status") {
          queue[i] = f;
          return;
        }
      }
    }
    queue.push(f);
  }

  function send(f: Frame) {
    if (f.event === "status") currentStatus = f;
    if (connected && sock) {
      sock.write(encodeFrame(f));
    } else {
      enqueue(f);
    }
  }

  function flush() {
    if (!sock || !connected) return;
    while (queue.length > 0) {
      const f = queue.shift()!;
      sock.write(encodeFrame(f));
    }
  }

  function scheduleReconnect() {
    if (stopped) return;
    const cap = Math.min(maxBackoff, initialBackoff * 2 ** reconnectAttempt);
    const delay = Math.floor(random() * cap); // full jitter
    reconnectAttempt++;
    reconnectTimer = setT(connect, delay);
  }

  function connect() {
    if (stopped) return;
    reconnectTimer = null;
    const s = connectFn(socketPath!);
    sock = s;
    s.on("connect", () => {
      connected = true;
      reconnectAttempt = 0;
      s.write(encodeFrame(helloFrame()));
      if (currentStatus) {
        // A status may have been buffered while disconnected; coalescing
        // guarantees at most one in the queue. Promote it to currentStatus.
        for (let i = queue.length - 1; i >= 0; i--) {
          if (queue[i].event === "status") {
            currentStatus = queue[i] as StatusFrame;
            queue.splice(i, 1);
            break;
          }
        }
        s.write(encodeFrame(currentStatus));
      }
      flush();
    });
    s.on("error", () => {
      // Ignore; 'close' will trigger reconnect.
    });
    s.on("close", () => {
      connected = false;
      sock = null;
      if (!stopped) scheduleReconnect();
    });
  }

  if (opts.bus) {
    opts.bus.on((e) => {
      switch (e.type) {
        case "session_start":
          send({ v: 1, event: "status", agent_id: agentId, status: "idle", ts: now() });
          break;
        case "agent_start":
          send({ v: 1, event: "status", agent_id: agentId, status: "working", task: e.task, ts: now() });
          break;
        case "tool_execution_start":
          send({ v: 1, event: "status", agent_id: agentId, status: "working", tool: e.tool, ts: now() });
          break;
        case "tool_execution_end":
          // Tool returned; pi is back to plain agent work until the next tool or agent_end.
          send({ v: 1, event: "status", agent_id: agentId, status: "working", ts: now() });
          break;
        case "tool_blocked":
          send({ v: 1, event: "status", agent_id: agentId, status: "blocked", tool: e.tool, ts: now() });
          break;
        case "agent_end":
          if (e.result) {
            send({ v: 1, event: "result", agent_id: agentId, text: e.result, ts: now() });
          }
          send({ v: 1, event: "status", agent_id: agentId, status: "idle", ts: now() });
          break;
        case "session_shutdown": {
          const bye: ByeFrame = { v: 1, event: "bye", agent_id: agentId, ts: now() };
          if (connected && sock) {
            sock.write(encodeFrame(bye));
            sock.end();
          } else {
            queue.push(bye);
          }
          stopped = true;
          if (reconnectTimer) clearT(reconnectTimer);
          break;
        }
      }
    });
  }

  connect();

  return {
    stop() {
      stopped = true;
      if (reconnectTimer) clearT(reconnectTimer);
      if (sock) sock.destroy();
    },
    _bufferSize: () => queue.length,
    _connected: () => connected,
  };
}

function extractFinalText(messages: unknown): string | undefined {
  if (!Array.isArray(messages)) return undefined;
  for (let i = messages.length - 1; i >= 0; i--) {
    const m = messages[i] as { role?: string; content?: unknown } | undefined;
    if (!m || m.role !== "assistant") continue;
    const c = m.content;
    if (typeof c === "string") return c;
    if (Array.isArray(c)) {
      const parts: string[] = [];
      for (const p of c) {
        if (p && typeof p === "object" && "type" in p && (p as { type: string }).type === "text") {
          const t = (p as { text?: unknown }).text;
          if (typeof t === "string") parts.push(t);
        }
      }
      if (parts.length) return parts.join("");
    }
    return undefined;
  }
  return undefined;
}

// Returning `handle` (not `void`) is harmless for pi's ExtensionFactory loader,
// and lets tests stop the socket layer deterministically.
export default function mureExtension(pi: ExtensionAPI): Handle {
  let emit: ((e: LifecycleEvent) => void) | undefined;
  const handle = start({
    piVersion: process.env.PI_VERSION ?? "unknown",
    bus: {
      on(handler) {
        emit = handler;
      },
    },
  });

  if (!mureEnabled(process.env)) return handle;

  pi.on("session_start", () => emit?.({ type: "session_start" }));
  pi.on("agent_start", () => emit?.({ type: "agent_start", task: process.env.MURE_TASK }));
  pi.on("tool_execution_start", (event) => {
    if (BLOCKING_TOOLS.has(event.toolName)) {
      emit?.({ type: "tool_blocked", tool: event.toolName });
    } else {
      emit?.({ type: "tool_execution_start", tool: event.toolName });
    }
  });
  pi.on("tool_execution_end", (event) => emit?.({ type: "tool_execution_end", tool: event.toolName }));
  pi.on("agent_end", (event) => emit?.({ type: "agent_end", result: extractFinalText(event?.messages) }));
  pi.on("session_shutdown", () => {
    emit?.({ type: "session_shutdown" });
    handle.stop();
  });

  pi.registerTool(makeMureSpawnTool(defaultExec, process.env));
  pi.registerTool(makeMureWaitTool(defaultExec, process.env));

  return handle;
}

export type ExecFn = (
  bin: string,
  argv: string[],
  opts: { env: NodeJS.ProcessEnv; timeoutMs?: number },
) => Promise<{ code: number; stdout: string; stderr: string; timedOut: boolean }>;

export const defaultExec: ExecFn = (bin, argv, opts) =>
  new Promise((resolve) => {
    // spawn (not execFile) to avoid the 1MiB maxBuffer cap on stdout —
    // `mure wait` payloads are unbounded by design.
    let stdout = "";
    let stderr = "";
    let timedOut = false;
    let killTimer: ReturnType<typeof setTimeout> | null = null;
    let timeoutTimer: ReturnType<typeof setTimeout> | null = null;
    let settled = false;
    const finish = (r: { code: number; stdout: string; stderr: string; timedOut: boolean }) => {
      if (settled) return;
      settled = true;
      if (timeoutTimer) clearTimeout(timeoutTimer);
      if (killTimer) clearTimeout(killTimer);
      resolve(r);
    };
    const child = spawn(bin, argv, { env: opts.env });
    child.stdout.on("data", (d) => { stdout += d.toString(); });
    child.stderr.on("data", (d) => { stderr += d.toString(); });
    child.on("error", (err) => finish({ code: -1, stdout: "", stderr: String(err), timedOut: false }));
    child.on("exit", (code, signal) => finish({ code: code ?? (signal ? -1 : 0), stdout, stderr, timedOut }));
    if (opts.timeoutMs !== undefined) {
      timeoutTimer = setTimeout(() => {
        timedOut = true;
        child.kill("SIGTERM");
        killTimer = setTimeout(() => child.kill("SIGKILL"), 2000);
      }, opts.timeoutMs);
    }
  });

export function resolveMureBin(env: NodeJS.ProcessEnv): string {
  return env.MURE_BIN || "mure";
}

export function parseSpawnStdout(stdout: string): { agent_id: string; pane_id: string } | null {
  const lines = stdout.split(/\r?\n/).filter((l) => l.trim() !== "");
  if (lines.length === 0) return null;
  const tokens = lines[lines.length - 1].split(/\s+/);
  if (tokens.length !== 2) return null;
  if (!tokens[1].startsWith("%")) return null;
  return { agent_id: tokens[0], pane_id: tokens[1] };
}

function okResult(text: string): AgentToolResult {
  return { content: [{ type: "text", text }] };
}
function errorResult(text: string): AgentToolResult {
  return { isError: true, content: [{ type: "text", text }] };
}

export function makeMureSpawnTool(exec: ExecFn, env: NodeJS.ProcessEnv): ToolDefinition<any> {
  return {
    name: "mure_spawn",
    label: "mure spawn",
    description:
      "Start a mure-managed coding-agent subagent in a new tmux pane. Pair with mure_wait to collect its result.",
    promptGuidelines: [
      "Use mure_spawn to start a coding-agent subagent in a new tmux pane. Pair with mure_wait to collect its result.",
    ],
    parameters: Type.Object({
      role: Type.String({ minLength: 1 }),
      task: Type.Optional(Type.String()),
    }),
    async execute(_id: string, { role, task }: { role: string; task?: string }) {
      const argv = task === undefined ? ["spawn", role] : ["spawn", role, task];
      const { code, stdout, stderr } = await exec(resolveMureBin(env), argv, { env });
      if (code !== 0) return errorResult(`mure spawn failed (exit ${code}): ${stderr.trim()}`);
      const parsed = parseSpawnStdout(stdout);
      if (!parsed) return errorResult(`mure_spawn: unparseable output: ${stdout}`);
      return okResult(JSON.stringify(parsed));
    },
  };
}

export function makeMureWaitTool(exec: ExecFn, env: NodeJS.ProcessEnv): ToolDefinition<any> {
  return {
    name: "mure_wait",
    label: "mure wait",
    description: "Block until a previously spawned mure agent emits its final result.",
    promptGuidelines: [
      "Use mure_wait to block until a previously spawned agent emits a result.",
    ],
    parameters: Type.Object({
      agent_id: Type.String({ minLength: 1 }),
      timeout_ms: Type.Optional(Type.Integer({ minimum: 1 })),
    }),
    async execute(_id: string, { agent_id, timeout_ms }: { agent_id: string; timeout_ms?: number }) {
      const ms = timeout_ms ?? 300_000;
      const { code, stdout, stderr, timedOut } = await exec(
        resolveMureBin(env),
        ["wait", agent_id],
        { env, timeoutMs: ms },
      );
      if (timedOut) return errorResult(`mure_wait timed out after ${ms}ms`);
      if (code !== 0) return errorResult(`mure wait failed (exit ${code}): ${stderr.trim()}`);
      return okResult(stdout.endsWith("\n") ? stdout.slice(0, -1) : stdout);
    },
  };
}
