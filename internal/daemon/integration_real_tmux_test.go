//go:build integration

package daemon

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/audibleblink/mure/internal/sock"
	"github.com/audibleblink/mure/internal/tmuxctl"
)

// TestRealTmuxStatusReachesPaneOption boots a real tmux server, starts the
// daemon against it, then verifies an agent's status write surfaces as a
// pane option via `tmux show-options -pv` within 1s.
func TestRealTmuxStatusReachesPaneOption(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	withSelfAuth(t)

	socket := "mure-test-" + filepath.Base(shortTempDir(t))
	tmux := func(args ...string) *exec.Cmd {
		return exec.Command("tmux", append([]string{"-L", socket}, args...)...)
	}
	if out, err := tmux("new-session", "-d", "-s", "test", "sh", "-c", "sleep 600").CombinedOutput(); err != nil {
		t.Fatalf("tmux new-session: %v: %s", err, out)
	}
	defer func() { _ = tmux("kill-server").Run() }()

	// Discover the only pane id.
	paneOut, err := tmux("list-panes", "-t", "test", "-F", "#{pane_id}").Output()
	if err != nil {
		t.Fatalf("list-panes: %v", err)
	}
	paneID := strings.TrimSpace(string(paneOut))
	if paneID == "" {
		t.Fatal("no pane id")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader, err := tmuxctl.Dial(ctx, "-L", socket, "attach", "-t", "test")
	if err != nil {
		t.Fatalf("reader dial: %v", err)
	}
	defer reader.Close()
	writer, err := tmuxctl.Dial(ctx, "-L", socket, "attach", "-t", "test")
	if err != nil {
		t.Fatalf("writer dial: %v", err)
	}
	defer writer.Close()

	runDir := shortTempDir(t)
	sockPath := filepath.Join(runDir, "d.sock")
	cfg := Config{
		Session:    "test",
		SocketPath: sockPath,
		RunDir:     runDir,
		Reader:     reader,
		Writer:     writer,
	}
	runErr := make(chan error, 1)
	go func() { runErr <- Run(ctx, cfg) }()

	// Wait for socket.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
		select {
		case e := <-runErr:
			t.Fatalf("daemon Run exited early: %v", e)
		default:
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(sockPath); err != nil {
		t.Fatalf("socket never appeared: %v", err)
	}

	// Agent stub.
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	_ = sock.WriteFrame(conn, sock.Hello{V: 1, Event: "hello", Role: sock.RoleAgent, AgentID: "a1", PaneID: paneID})
	_ = sock.WriteFrame(conn, sock.Status{V: 1, Event: "status", AgentID: "a1", Status: sock.StatusWorking, Task: "x", TS: 1})

	waitFor(t, 2*time.Second, "@mure-status==working", func() bool {
		out, err := tmux("show-options", "-pv", "-t", paneID, "@mure-status").Output()
		if err != nil {
			return false
		}
		return strings.TrimSpace(string(out)) == "working"
	})
}
