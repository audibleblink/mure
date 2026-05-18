package main

import (
	"strings"
	"testing"
	"time"

	"github.com/audibleblink/mure/internal/sock"
)

func TestWaitReturnsResult(t *testing.T) {
	sockPath, roster := startInProcessDaemon(t)
	roster.UpsertFromHello(sock.Hello{V: 1, Event: "hello", Role: sock.RoleAgent, AgentID: "agent-w", PaneID: "%1"})
	t.Setenv("MURE_SOCKET", sockPath)

	resCh := make(chan struct {
		exit int
		out  string
	}, 1)
	go func() {
		exit, out, _ := captureRun(t, []string{"wait", "agent-w"})
		resCh <- struct {
			exit int
			out  string
		}{exit, out}
	}()

	time.Sleep(50 * time.Millisecond)
	roster.ApplyResult(sock.Result{V: 1, Event: "result", AgentID: "agent-w", Text: "the answer", TS: 1})

	select {
	case r := <-resCh:
		if r.exit != 0 {
			t.Fatalf("exit=%d", r.exit)
		}
		if strings.TrimSpace(r.out) != "the answer" {
			t.Fatalf("stdout=%q", r.out)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("wait did not return")
	}
}
