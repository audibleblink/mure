//go:build e2e

// Throughput test (PRD §13): with 8 panes each running `yes`, daemon
// attached and %output suppressed, measure p99 latency from agent
// status send to pane-option observable via show-options.
//
// The default coalescer window is 500ms; for this latency-budget test we
// configure a small window (10ms) via daemon.Config.CoalesceWindow so we
// measure the daemon's reader→writer hot path rather than the batching
// delay. The PRD budget of <100ms covers everything except the coalesce
// dwell time.
package throughput_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/audibleblink/mure/internal/daemon"
	"github.com/audibleblink/mure/internal/sock"
	"github.com/audibleblink/mure/internal/tmuxctl"
)

func TestThroughputP99(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	// Self-auth: skip SO_PEERCRED check for in-process clients.
	origCheck := daemon.Check
	daemon.Check = func(net.Conn) error { return nil }
	t.Cleanup(func() { daemon.Check = origCheck })

	tmp, err := os.MkdirTemp("/tmp", "mure-thr-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })

	tmuxSock := filepath.Join(tmp, "tmux.sock")
	mureSock := filepath.Join(tmp, "mure.sock")
	session := "thr"

	tmuxCmd := func(args ...string) *exec.Cmd {
		return exec.Command("tmux", append([]string{"-S", tmuxSock}, args...)...)
	}

	if out, err := tmuxCmd("new-session", "-d", "-s", session, "-x", "800", "-y", "400", "sh", "-c", "yes >/dev/null").CombinedOutput(); err != nil {
		t.Fatalf("new-session: %v: %s", err, out)
	}
	t.Cleanup(func() { _ = tmuxCmd("kill-server").Run() })

	// Spawn 7 more panes running `yes`, each in a new window to avoid
	// per-window pane size limits.
	for i := 0; i < 7; i++ {
		if out, err := tmuxCmd("new-window", "-t", session, "-d", "sh", "-c", "yes >/dev/null").CombinedOutput(); err != nil {
			t.Fatalf("new-window %d: %v: %s", i, err, out)
		}
	}
	// Collect pane ids.
	out, err := tmuxCmd("list-panes", "-s", "-t", session, "-F", "#{pane_id}").Output()
	if err != nil {
		t.Fatalf("list-panes: %v", err)
	}
	var panes []string
	for _, line := range strings.Fields(string(out)) {
		panes = append(panes, line)
	}
	if len(panes) != 8 {
		t.Fatalf("want 8 panes, got %d", len(panes))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, err := tmuxctl.Dial(ctx, "-S", tmuxSock, "attach", "-t", session)
	if err != nil {
		t.Fatalf("reader dial: %v", err)
	}
	defer reader.Close()
	writer, err := tmuxctl.Dial(ctx, "-S", tmuxSock, "attach", "-t", session)
	if err != nil {
		t.Fatalf("writer dial: %v", err)
	}
	defer writer.Close()

	runDir := tmp
	cfg := daemon.Config{
		Session:        session,
		SocketPath:     mureSock,
		RunDir:         runDir,
		Reader:         reader,
		Writer:         writer,
		CoalesceWindow: 10 * time.Millisecond,
	}
	runErr := make(chan error, 1)
	go func() { runErr <- daemon.Run(ctx, cfg) }()

	// Wait for socket.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(mureSock); err == nil {
			break
		}
		select {
		case e := <-runErr:
			t.Fatalf("daemon Run exited early: %v", e)
		default:
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Open one agent connection per pane, send hello.
	type ag struct {
		id   string
		pane string
		conn net.Conn
	}
	var agents []ag
	for i, p := range panes {
		c, err := net.Dial("unix", mureSock)
		if err != nil {
			t.Fatalf("agent dial %d: %v", i, err)
		}
		id := fmt.Sprintf("a%d", i)
		if err := sock.WriteFrame(c, sock.Hello{
			V: 1, Event: "hello", Role: sock.RoleAgent,
			AgentID: id, PaneID: p, PID: i + 1, TS: 1,
		}); err != nil {
			t.Fatalf("hello: %v", err)
		}
		agents = append(agents, ag{id: id, pane: p, conn: c})
	}
	t.Cleanup(func() {
		for _, a := range agents {
			_ = a.conn.Close()
		}
	})

	// Measure: for each round, alternate status, sample show-options
	// until the new value appears, record latency.
	const rounds = 30
	want := []string{sock.StatusWorking, sock.StatusIdle}
	var samples []time.Duration
	for r := 0; r < rounds; r++ {
		st := want[r%2]
		// Send status frame to each agent close together.
		sendT := time.Now()
		for _, a := range agents {
			if err := sock.WriteFrame(a.conn, sock.Status{
				V: 1, Event: "status",
				AgentID: a.id, Status: st, TS: sendT.UnixMilli(),
			}); err != nil {
				t.Fatalf("status: %v", err)
			}
		}
		// For each pane, poll show-options until it reflects.
		for _, a := range agents {
			ok := false
			for time.Since(sendT) < 2*time.Second {
				v, err := tmuxCmd("show-options", "-pv", "-t", a.pane, "@mure-status").Output()
				if err == nil && strings.TrimSpace(string(v)) == st {
					samples = append(samples, time.Since(sendT))
					ok = true
					break
				}
				time.Sleep(2 * time.Millisecond)
			}
			if !ok {
				t.Fatalf("round %d pane %s never saw %s", r, a.pane, st)
			}
		}
	}

	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	if len(samples) == 0 {
		t.Fatal("no samples")
	}
	p99 := samples[(len(samples)*99)/100]
	p50 := samples[len(samples)/2]
	t.Logf("n=%d p50=%v p99=%v max=%v", len(samples), p50, p99, samples[len(samples)-1])
	if p99 > 100*time.Millisecond {
		t.Fatalf("p99 latency %v exceeds 100ms budget (PRD §13)", p99)
	}
	_ = errors.New
}
