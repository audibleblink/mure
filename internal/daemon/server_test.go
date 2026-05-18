package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/audibleblink/mure/internal/sock"
)

// withSelfAuth replaces the peer-auth check with a permissive stub for the
// duration of the test.
func withSelfAuth(t *testing.T) {
	t.Helper()
	orig := Check
	Check = func(net.Conn) error { return nil }
	t.Cleanup(func() { Check = orig })
}

// shortTempDir returns a t.Cleanup-tracked dir whose path is short enough for
// macOS' sockaddr_un sun_path (~104 chars).
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "mure-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func startTestServer(t *testing.T) (*Server, *Roster, string, context.CancelFunc) {
	t.Helper()
	withSelfAuth(t)
	dir := shortTempDir(t)
	sockPath := filepath.Join(dir, "m.sock")
	roster := NewRoster()
	srv, err := Listen(sockPath, roster)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		roster.Close()
	})
	return srv, roster, sockPath, cancel
}

func dial(t *testing.T, path string) net.Conn {
	t.Helper()
	c, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return c
}

func TestServerAgentHelloStatusPropagatesToSidebar(t *testing.T) {
	_, _, sockPath, _ := startTestServer(t)

	// Sidebar subscriber.
	sb := dial(t, sockPath)
	defer sb.Close()
	if err := sock.WriteFrame(sb, sock.Hello{V: 1, Event: "hello", Role: sock.RoleSidebar}); err != nil {
		t.Fatal(err)
	}
	sbr := bufio.NewReader(sb)
	// First frame is the initial roster snapshot (empty).
	if _, err := sock.ReadFrame(sbr, sock.MaxFrameSize); err != nil {
		t.Fatalf("read initial roster: %v", err)
	}

	// Agent connection.
	ag := dial(t, sockPath)
	defer ag.Close()
	hello := sock.Hello{V: 1, Event: "hello", Role: sock.RoleAgent, AgentID: "a1", PaneID: "%41"}
	if err := sock.WriteFrame(ag, hello); err != nil {
		t.Fatal(err)
	}
	status := sock.Status{V: 1, Event: "status", AgentID: "a1", Status: sock.StatusWorking, Task: "build", TS: 99}
	if err := sock.WriteFrame(ag, status); err != nil {
		t.Fatal(err)
	}

	// Sidebar should receive: agent_update (idle from hello), agent_update (working from status).
	gotWorking := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_ = sb.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		line, err := sock.ReadFrame(sbr, sock.MaxFrameSize)
		if err != nil {
			continue
		}
		var upd sock.AgentUpdate
		if json.Unmarshal(line, &upd) != nil {
			continue
		}
		if upd.Agent.ID == "a1" && upd.Agent.Status == sock.StatusWorking {
			gotWorking = true
			break
		}
	}
	if !gotWorking {
		t.Fatal("sidebar never saw working status for a1")
	}
}

func TestServerRejectsNonHelloFirstFrame(t *testing.T) {
	_, _, sockPath, _ := startTestServer(t)
	c := dial(t, sockPath)
	defer c.Close()
	// Send a 'status' as first frame — server must close.
	if err := sock.WriteFrame(c, sock.Status{V: 1, Event: "status", AgentID: "a", Status: "idle"}); err != nil {
		t.Fatal(err)
	}
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1)
	_, err := c.Read(buf)
	if err == nil {
		t.Fatal("expected connection to be closed by server")
	}
}

func TestServerRejectsOversizedFrame(t *testing.T) {
	_, _, sockPath, _ := startTestServer(t)
	c := dial(t, sockPath)
	defer c.Close()
	huge := strings.Repeat("x", sock.MaxFrameSize+1)
	if _, err := c.Write([]byte(huge + "\n")); err != nil {
		t.Fatal(err)
	}
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1)
	if _, err := c.Read(buf); err == nil {
		t.Fatal("expected server to close on oversized frame")
	}
}

func TestServerPeerAuthRejection(t *testing.T) {
	dir := shortTempDir(t)
	sockPath := filepath.Join(dir, "m.sock")
	rosterX := NewRoster()
	defer rosterX.Close()

	orig := Check
	Check = func(net.Conn) error { return ErrPeerNotSelf }
	t.Cleanup(func() { Check = orig })

	srv, err := Listen(sockPath, rosterX)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Run(ctx) }()

	c := dial(t, sockPath)
	defer c.Close()
	// Even a valid hello must be rejected.
	_ = sock.WriteFrame(c, sock.Hello{V: 1, Event: "hello", Role: sock.RoleAgent, AgentID: "a"})
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1)
	if _, err := c.Read(buf); err == nil {
		t.Fatal("expected immediate close on auth rejection")
	}
}

func TestStaleSocketUnlinked(t *testing.T) {
	withSelfAuth(t)
	dir := shortTempDir(t)
	sockPath := filepath.Join(dir, "m.sock")

	// Stale scenario: an existing file at the path with no listener.
	if err := os.WriteFile(sockPath, []byte("not a socket"), 0o600); err != nil {
		t.Fatal(err)
	}

	rosterX := NewRoster()
	defer rosterX.Close()
	srv, err := Listen(sockPath, rosterX)
	if err != nil {
		t.Fatalf("expected stale socket to be cleaned up, got: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Run(ctx) }()

	// Should be able to dial now.
	c, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial after cleanup: %v", err)
	}
	c.Close()
}

func TestStaleSocketAliveRefusesSecondDaemon(t *testing.T) {
	withSelfAuth(t)
	dir := shortTempDir(t)
	sockPath := filepath.Join(dir, "m.sock")
	rosterX := NewRoster()
	defer rosterX.Close()
	srv, err := Listen(sockPath, rosterX)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Run(ctx) }()

	// Second daemon attempt must refuse.
	rosterY := NewRoster()
	defer rosterY.Close()
	if _, err := Listen(sockPath, rosterY); err == nil {
		t.Fatal("expected second Listen to fail with live daemon")
	} else if !strings.Contains(err.Error(), "already running") {
		t.Fatalf("expected 'already running' error, got: %v", err)
	}
}
