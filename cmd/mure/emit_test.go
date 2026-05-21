package main

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/audibleblink/mure/internal/sock"
)

// fakeDaemon listens on a unix socket, accepts one connection, reads frames
// until close, and stores them in Frames (decoded as map[string]any).
type fakeDaemon struct {
	mu     sync.Mutex
	frames []map[string]any
	done   chan struct{}
}

func startFakeDaemon(t *testing.T) (string, *fakeDaemon) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "d.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &fakeDaemon{done: make(chan struct{})}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		defer close(f.done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		for {
			line, err := sock.ReadFrame(br, sock.MaxFrameSize)
			if err != nil {
				return
			}
			var m map[string]any
			if err := json.Unmarshal(line, &m); err != nil {
				return
			}
			f.mu.Lock()
			f.frames = append(f.frames, m)
			f.mu.Unlock()
		}
	}()
	return path, f
}

func (f *fakeDaemon) wait(t *testing.T) []map[string]any {
	t.Helper()
	select {
	case <-f.done:
	case <-time.After(2 * time.Second):
		t.Fatal("fakeDaemon never closed")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.frames...)
}

func TestEmitStatusFrame(t *testing.T) {
	path, fd := startFakeDaemon(t)
	t.Setenv("MURE_SOCKET", path)
	t.Setenv("MURE_AGENT_ID", "agent-x")
	t.Setenv("MURE_PANE_ID", "%9")
	t.Setenv("TMUX", "")

	exit, _, errs := captureRun(t, []string{"emit", "status", "working", "--tool", "foo"})
	if exit != 0 {
		t.Fatalf("exit=%d errs=%q", exit, errs)
	}
	frames := fd.wait(t)
	if len(frames) != 2 {
		t.Fatalf("got %d frames, want 2: %+v", len(frames), frames)
	}
	hello := frames[0]
	if hello["event"] != "hello" || hello["role"] != "agent" || hello["agent_id"] != "agent-x" ||
		hello["pane_id"] != "%9" || hello["oneshot"] != true {
		t.Fatalf("hello bad: %+v", hello)
	}
	st := frames[1]
	if st["event"] != "status" || st["agent_id"] != "agent-x" || st["status"] != "working" || st["tool"] != "foo" {
		t.Fatalf("status bad: %+v", st)
	}
	if _, ok := st["ts"].(float64); !ok {
		t.Fatalf("ts missing: %+v", st)
	}
}

func TestEmitResultFromStdin(t *testing.T) {
	path, fd := startFakeDaemon(t)
	t.Setenv("MURE_SOCKET", path)
	t.Setenv("MURE_AGENT_ID", "agent-r")
	t.Setenv("TMUX", "")

	// Redirect stdin via os.Stdin replacement.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })
	go func() {
		_, _ = w.Write([]byte("final answer\n"))
		w.Close()
	}()
	exit, _, errs := captureRun(t, []string{"emit", "result", "-"})
	if exit != 0 {
		t.Fatalf("exit=%d errs=%q", exit, errs)
	}
	frames := fd.wait(t)
	if len(frames) != 2 || frames[1]["event"] != "result" || frames[1]["text"] != "final answer\n" {
		t.Fatalf("frames=%+v", frames)
	}
}

func TestEmitMissingSocket(t *testing.T) {
	t.Setenv("MURE_SOCKET", "")
	t.Setenv("MURE_AGENT_ID", "agent-x")
	exit, _, errs := captureRun(t, []string{"emit", "status", "working"})
	if exit == 0 {
		t.Fatalf("expected non-zero exit")
	}
	if !strings.Contains(errs, "MURE_SOCKET") {
		t.Fatalf("stderr=%q", errs)
	}
}
