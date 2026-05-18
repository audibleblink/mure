package daemon

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/audibleblink/mure/internal/sock"
	"github.com/audibleblink/mure/internal/tmuxctl"
)

func waitFor(t *testing.T, d time.Duration, msg string, pred func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if pred() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for: %s", msg)
}

func TestBridgePaneDiedRemovesAgent(t *testing.T) {
	roster := NewRoster()
	defer roster.Close()
	coal := NewCoalescer(50 * time.Millisecond)
	defer coal.Close()
	reader := tmuxctl.NewScriptedFake()
	defer reader.Close()
	writer := tmuxctl.NewScriptedFake()
	defer writer.Close()
	// Pre-queue replies for any incidental writes (e.g. coalesced @mure-status from initial hello).
	for i := 0; i < 16; i++ {
		writer.EnqueueReply("", nil)
	}
	deb := NewDebouncer(50*time.Millisecond,
		func(id string) { roster.Remove(id) },
		func(id string) { roster.MarkDisconnected(id, 0) })
	defer deb.Stop()

	roster.UpsertFromHello(sock.Hello{V: 1, Event: "hello", Role: sock.RoleAgent, AgentID: "a1", PaneID: "%41"})

	b := NewBridge(reader, writer, roster, coal, deb, "test")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go b.Run(ctx)

	// Give bridge a moment to subscribe before we emit the event.
	time.Sleep(20 * time.Millisecond)

	reader.EmitEvent(tmuxctl.Event{Kind: tmuxctl.EventPaneDied, PaneID: "%41"})

	waitFor(t, time.Second, "a1 removed", func() bool {
		for _, a := range roster.Snapshot().Agents {
			if a.ID == "a1" {
				return false
			}
		}
		return true
	})
}

func TestBridgeCoalescesStatusWritesToTmux(t *testing.T) {
	roster := NewRoster()
	defer roster.Close()
	coal := NewCoalescer(100 * time.Millisecond)
	defer coal.Close()
	reader := tmuxctl.NewScriptedFake()
	defer reader.Close()
	writer := tmuxctl.NewScriptedFake()
	defer writer.Close()
	for i := 0; i < 32; i++ {
		writer.EnqueueReply("", nil)
	}

	deb := NewDebouncer(time.Second, func(string) {}, func(string) {})
	defer deb.Stop()

	b := NewBridge(reader, writer, roster, coal, deb, "test")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go b.Run(ctx)
	time.Sleep(20 * time.Millisecond) // let subscriber register

	roster.UpsertFromHello(sock.Hello{V: 1, Event: "hello", Role: sock.RoleAgent, AgentID: "a1", PaneID: "%41"})
	for i := 0; i < 5; i++ {
		roster.ApplyStatus(sock.Status{V: 1, Event: "status", AgentID: "a1", Status: sock.StatusWorking, Task: "build", TS: int64(i)})
	}

	// Wait for coalescer to flush + writer to drain.
	time.Sleep(400 * time.Millisecond)

	cmds := writer.Commands()
	statusCmds := 0
	var lastStatusCmd string
	for _, c := range cmds {
		if strings.Contains(c, "@mure-status") {
			statusCmds++
			lastStatusCmd = c
		}
	}
	if statusCmds != 1 {
		t.Fatalf("expected exactly 1 @mure-status write, got %d (cmds=%v)", statusCmds, cmds)
	}
	wantPrefix := "set-option -p -t %41 @mure-status "
	if !strings.HasPrefix(lastStatusCmd, wantPrefix) || !strings.Contains(lastStatusCmd, "working") {
		t.Fatalf("unexpected cmd: %q", lastStatusCmd)
	}
}

func TestBridgeEscapesHashInTmuxOptionValues(t *testing.T) {
	roster := NewRoster()
	defer roster.Close()
	coal := NewCoalescer(20 * time.Millisecond)
	defer coal.Close()
	reader := tmuxctl.NewScriptedFake()
	defer reader.Close()
	writer := tmuxctl.NewScriptedFake()
	defer writer.Close()
	for i := 0; i < 8; i++ {
		writer.EnqueueReply("", nil)
	}
	deb := NewDebouncer(time.Second, func(string) {}, func(string) {})
	defer deb.Stop()

	b := NewBridge(reader, writer, roster, coal, deb, "test")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go b.Run(ctx)
	time.Sleep(20 * time.Millisecond)

	roster.UpsertFromHello(sock.Hello{V: 1, Event: "hello", Role: sock.RoleAgent, AgentID: "a1", PaneID: "%41"})
	// Task value contains tmux format directives that would be re-expanded
	// by pane-border-format if not escaped.
	roster.ApplyStatus(sock.Status{V: 1, Event: "status", AgentID: "a1", Status: sock.StatusWorking, Task: "#(id)#{e:kill}", TS: 1})

	time.Sleep(120 * time.Millisecond)

	var taskCmd string
	for _, c := range writer.Commands() {
		if strings.Contains(c, "@mure-task") {
			taskCmd = c
		}
	}
	if taskCmd == "" {
		t.Fatalf("no @mure-task write seen; cmds=%v", writer.Commands())
	}
	if !strings.Contains(taskCmd, "##(id)##{e:kill}") {
		t.Fatalf("expected '#' escaped to '##' in task value; got: %q", taskCmd)
	}
	if strings.Contains(taskCmd, "'#(") || strings.Contains(taskCmd, "'#{") {
		t.Fatalf("raw unescaped '#(' or '#{' leaked into tmux cmd: %q", taskCmd)
	}
}
