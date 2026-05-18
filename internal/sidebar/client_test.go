package sidebar

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/audibleblink/mure/internal/sock"
)

// stubServer is a minimal sidebar-facing daemon: accepts a hello, then sends
// the rosters provided to send().
type stubServer struct {
	ln    net.Listener
	t     *testing.T
	conns chan net.Conn
	stop  chan struct{}
}

func startStub(t *testing.T, path string) *stubServer {
	t.Helper()
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	s := &stubServer{ln: ln, t: t, conns: make(chan net.Conn, 4), stop: make(chan struct{})}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			// Read hello.
			br := bufio.NewReader(c)
			if _, err := sock.ReadFrame(br, sock.MaxFrameSize); err != nil {
				_ = c.Close()
				continue
			}
			select {
			case s.conns <- c:
			case <-s.stop:
				_ = c.Close()
				return
			}
		}
	}()
	return s
}

func (s *stubServer) sendRoster(c net.Conn, ids ...string) {
	agents := make([]sock.AgentSnapshot, 0, len(ids))
	for _, id := range ids {
		agents = append(agents, sock.AgentSnapshot{ID: id, Status: sock.StatusIdle})
	}
	r := sock.Roster{V: 1, Event: "roster", Agents: agents}
	b, _ := json.Marshal(r)
	b = append(b, '\n')
	_, _ = c.Write(b)
}

func (s *stubServer) close() {
	close(s.stop)
	_ = s.ln.Close()
}

func TestClient_ReconnectAfterServerKill(t *testing.T) {
	dir, err := os.MkdirTemp("", "mure")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "s")

	srv := startStub(t, path)

	c := NewClient(path)
	c.BaseDelay = 5 * time.Millisecond
	c.MaxDelay = 20 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go c.Run(ctx)

	// First connection.
	var conn1 net.Conn
	select {
	case conn1 = <-srv.conns:
	case <-time.After(time.Second):
		t.Fatal("no connection 1")
	}
	srv.sendRoster(conn1, "a1", "a2")

	waitFor := func(pred func(Frame) bool, desc string) Frame {
		t.Helper()
		deadline := time.After(2 * time.Second)
		for {
			select {
			case f, ok := <-c.Frames:
				if !ok {
					t.Fatalf("frames closed waiting for %s", desc)
				}
				if pred(f) {
					return f
				}
			case <-deadline:
				t.Fatalf("timeout waiting for %s", desc)
			}
		}
	}

	f := waitFor(func(f Frame) bool { return f.Roster != nil }, "first roster")
	if len(f.Roster.Agents) != 2 {
		t.Fatalf("want 2 agents, got %d", len(f.Roster.Agents))
	}

	// Kill server. Client should emit disconnect, then reconnect.
	_ = conn1.Close()
	srv.close()

	waitFor(func(f Frame) bool { return f.Roster == nil && f.Update == nil && !f.Connect }, "disconnect frame")

	// Restart server on same path.
	srv2 := startStub(t, path)
	defer srv2.close()

	var conn2 net.Conn
	select {
	case conn2 = <-srv2.conns:
	case <-time.After(2 * time.Second):
		t.Fatal("no reconnect")
	}
	srv2.sendRoster(conn2, "b1")

	f = waitFor(func(f Frame) bool { return f.Roster != nil }, "second roster")
	if len(f.Roster.Agents) != 1 || f.Roster.Agents[0].ID != "b1" {
		t.Fatalf("unexpected second roster: %+v", f.Roster)
	}
}
