package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/audibleblink/mure/internal/daemon"
	"github.com/audibleblink/mure/internal/sock"
)

// startInProcessDaemon spins up a daemon on a temp Unix socket and returns
// its path and the roster (so tests can seed agents directly).
func startInProcessDaemon(t *testing.T) (string, *daemon.Roster) {
	t.Helper()
	// Bypass peer-auth in tests (cross-process self UID checks are fine on
	// localhost, but we keep it explicit so the harness mirrors daemon_test).
	daemon.Check = func(net.Conn) error { return nil }

	dir, err := os.MkdirTemp("/tmp", "mure-cli-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	sockPath := filepath.Join(dir, "d.sock")
	roster := daemon.NewRoster()
	srv, err := daemon.Listen(sockPath, roster)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		roster.Close()
	})
	return sockPath, roster
}

// captureRun invokes the CLI dispatcher with argv and returns (exit, stdout, stderr).
func captureRun(t *testing.T, argv []string) (int, string, string) {
	t.Helper()
	outR, outW, _ := os.Pipe()
	errR, errW, _ := os.Pipe()
	done := make(chan int, 1)
	go func() { done <- run(argv, outW, errW) }()
	exit := <-done
	outW.Close()
	errW.Close()
	var outBuf, errBuf bytes.Buffer
	_, _ = outBuf.ReadFrom(outR)
	_, _ = errBuf.ReadFrom(errR)
	return exit, outBuf.String(), errBuf.String()
}

func TestUpReEntrancyAlreadyRunning(t *testing.T) {
	sockPath, _ := startInProcessDaemon(t)
	t.Setenv("MURE_SOCKET", sockPath)
	exit, out, _ := captureRun(t, []string{"up"})
	if exit != 0 {
		t.Fatalf("exit=%d", exit)
	}
	if want := "already running"; !contains(out, want) {
		t.Fatalf("stdout=%q does not contain %q", out, want)
	}
}

func TestLsJSON(t *testing.T) {
	sockPath, roster := startInProcessDaemon(t)
	roster.UpsertFromHello(sock.Hello{V: 1, Event: "hello", Role: sock.RoleAgent, AgentID: "agent-1", PaneID: "%41"})
	roster.ApplyStatus(sock.Status{V: 1, Event: "status", AgentID: "agent-1", Status: sock.StatusWorking, Task: "build", TS: 7})
	// Allow roster to apply.
	time.Sleep(50 * time.Millisecond)

	t.Setenv("MURE_SOCKET", sockPath)
	exit, out, errs := captureRun(t, []string{"ls", "--json"})
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, errs)
	}
	var r sock.Roster
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("unmarshal: %v\nstdout=%q", err, out)
	}
	if r.Event != "roster" || len(r.Agents) != 1 || r.Agents[0].ID != "agent-1" || r.Agents[0].Status != sock.StatusWorking {
		t.Fatalf("unexpected roster: %+v", r)
	}
}

func TestLsTable(t *testing.T) {
	sockPath, roster := startInProcessDaemon(t)
	roster.UpsertFromHello(sock.Hello{V: 1, Event: "hello", Role: sock.RoleAgent, AgentID: "agent-2", PaneID: "%9"})
	time.Sleep(50 * time.Millisecond)
	t.Setenv("MURE_SOCKET", sockPath)
	exit, out, _ := captureRun(t, []string{"ls"})
	if exit != 0 {
		t.Fatalf("exit=%d", exit)
	}
	if !contains(out, "AGENT") || !contains(out, "agent-2") || !contains(out, "idle") {
		t.Fatalf("unexpected ls output:\n%s", out)
	}
}

func TestDownSendsShutdown(t *testing.T) {
	sockPath, _ := startInProcessDaemon(t)
	t.Setenv("MURE_SOCKET", sockPath)
	exit, _, errs := captureRun(t, []string{"down"})
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, errs)
	}
	// Daemon should have unlinked the socket within a short window.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sockPath); os.IsNotExist(err) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("socket not unlinked: %s", sockPath)
}

func TestDoctorNoTmuxFails(t *testing.T) {
	// Empty PATH means tmux can't be found.
	t.Setenv("PATH", "/nonexistent")
	exit, out, errs := captureRun(t, []string{"doctor"})
	if exit == 0 {
		t.Fatalf("expected non-zero exit, got 0\nstdout=%s\nstderr=%s", out, errs)
	}
	if !contains(out, "tmux") {
		t.Fatalf("expected tmux message, got:\n%s", out)
	}
}

func TestIntegrationInstallUninstall(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "mure-pi-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	t.Setenv("PI_CODING_AGENT_DIR", dir)

	exit, _, errs := captureRun(t, []string{"integration", "install", "pi"})
	if exit != 0 {
		t.Fatalf("install exit=%d stderr=%s", exit, errs)
	}
	want := []string{"package.json", "index.ts"}
	for _, f := range want {
		p := filepath.Join(dir, "extensions", "mure", f)
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("missing %s: %v", p, err)
		}
	}

	exit, _, _ = captureRun(t, []string{"integration", "uninstall", "pi"})
	if exit != 0 {
		t.Fatalf("uninstall exit=%d", exit)
	}
	if _, err := os.Stat(filepath.Join(dir, "extensions", "mure")); !os.IsNotExist(err) {
		t.Fatalf("extension dir should be gone: %v", err)
	}
}

func TestUnknownVerb(t *testing.T) {
	exit, _, errs := captureRun(t, []string{"nope"})
	if exit != 2 {
		t.Fatalf("exit=%d errs=%s", exit, errs)
	}
}

func contains(s, sub string) bool { return bytes.Contains([]byte(s), []byte(sub)) }
