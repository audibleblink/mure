// Tiny test helper: a Unix-socket server that captures NDJSON frames.
import net from "node:net";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

export interface CapturedConn {
  frames: string[];
  socket: net.Socket;
  done: Promise<void>;
}

export interface TestServer {
  path: string;
  conns: CapturedConn[];
  waitForConn(i: number): Promise<CapturedConn>;
  waitForFrame(i: number, n: number): Promise<void>;
  drop(i: number): void;
  close(): Promise<void>;
}

export async function makeServer(): Promise<TestServer> {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "mure-test-"));
  const p = path.join(dir, "daemon.sock");
  const conns: CapturedConn[] = [];
  const waiters: Array<(c: CapturedConn) => void> = [];

  const server = net.createServer((sock) => {
    let buf = "";
    const frames: string[] = [];
    let resolveDone!: () => void;
    const done = new Promise<void>((r) => (resolveDone = r));
    const c: CapturedConn = { frames, socket: sock, done };
    sock.on("data", (chunk) => {
      buf += chunk.toString("utf8");
      let nl;
      while ((nl = buf.indexOf("\n")) >= 0) {
        frames.push(buf.slice(0, nl));
        buf = buf.slice(nl + 1);
      }
    });
    sock.on("close", () => resolveDone());
    sock.on("error", () => {});
    conns.push(c);
    const w = waiters.shift();
    if (w) w(c);
  });

  await new Promise<void>((resolve, reject) => {
    server.once("error", reject);
    server.listen(p, () => resolve());
  });

  return {
    path: p,
    conns,
    waitForConn(i) {
      if (conns[i]) return Promise.resolve(conns[i]);
      return new Promise((resolve) => waiters.push(resolve));
    },
    async waitForFrame(i, n) {
      const c = await this.waitForConn(i);
      while (c.frames.length < n) {
        await new Promise((r) => setTimeout(r, 5));
      }
    },
    drop(i) {
      conns[i]?.socket.destroy();
    },
    async close() {
      await new Promise<void>((r) => server.close(() => r()));
      try {
        fs.rmSync(dir, { recursive: true, force: true });
      } catch {}
    },
  };
}

export class FakeBus {
  private handler?: (e: any) => void;
  on(h: (e: any) => void) {
    this.handler = h;
  }
  emit(e: any) {
    this.handler?.(e);
  }
}
