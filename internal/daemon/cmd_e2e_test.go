package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/audibleblink/mure/internal/sock"
)

// TestE2ERosterPropagation: in-process agent stub + sidebar stub, full
// roundtrip through the Unix socket. No tmux involved.
func TestE2ERosterPropagation(t *testing.T) {
	withSelfAuth(t)
	dir := shortTempDir(t)
	sockPath := filepath.Join(dir, "m.sock")
	roster := NewRoster()
	defer roster.Close()
	srv, err := Listen(sockPath, roster)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Run(ctx) }()

	// Sidebar stub
	sbConn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer sbConn.Close()
	if err := sock.WriteFrame(sbConn, sock.Hello{V: 1, Event: "hello", Role: sock.RoleSidebar}); err != nil {
		t.Fatal(err)
	}
	sbr := bufio.NewReader(sbConn)
	// Initial roster (empty)
	if _, err := sock.ReadFrame(sbr, sock.MaxFrameSize); err != nil {
		t.Fatalf("initial roster: %v", err)
	}

	// Agent stub
	agConn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer agConn.Close()
	if err := sock.WriteFrame(agConn, sock.Hello{V: 1, Event: "hello", Role: sock.RoleAgent, AgentID: "agent-1", PaneID: "%7", PID: 1234}); err != nil {
		t.Fatal(err)
	}
	if err := sock.WriteFrame(agConn, sock.Status{V: 1, Event: "status", AgentID: "agent-1", Status: sock.StatusWorking, Task: "compile", TS: 1}); err != nil {
		t.Fatal(err)
	}
	if err := sock.WriteFrame(agConn, sock.Status{V: 1, Event: "status", AgentID: "agent-1", Status: sock.StatusIdle, TS: 2}); err != nil {
		t.Fatal(err)
	}

	// Expect: hello upsert (idle), working, idle. Order preserved.
	wantSeq := []string{sock.StatusIdle, sock.StatusWorking, sock.StatusIdle}
	for i, want := range wantSeq {
		_ = sbConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		line, err := sock.ReadFrame(sbr, sock.MaxFrameSize)
		if err != nil {
			t.Fatalf("step %d read: %v", i, err)
		}
		var upd sock.AgentUpdate
		if err := json.Unmarshal(line, &upd); err != nil {
			t.Fatalf("step %d unmarshal: %v", i, err)
		}
		if upd.Agent.ID != "agent-1" || upd.Agent.Status != want {
			t.Fatalf("step %d: got id=%q status=%q, want status=%q", i, upd.Agent.ID, upd.Agent.Status, want)
		}
	}

	// Final snapshot via roster.
	snap := roster.Snapshot()
	if len(snap.Agents) != 1 || snap.Agents[0].ID != "agent-1" || snap.Agents[0].Status != sock.StatusIdle {
		t.Fatalf("unexpected final snapshot: %+v", snap)
	}
}
