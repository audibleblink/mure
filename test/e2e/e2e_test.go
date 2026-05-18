//go:build e2e

// End-to-end test for mure: real tmux server, real `mure` binary,
// real stub agents speaking the socket protocol.
package e2e_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/audibleblink/mure/internal/sock"
)

const (
	tmuxSession = "mure-e2e"
)

type harness struct {
	t        *testing.T
	repoRoot string
	tmpDir   string
	mure     string
	stub     string
	tmuxSock string
	mureSock string
	tmuxEnv  []string
	mureEnv  []string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	root := repoRoot(t)
	tmp, err := os.MkdirTemp("/tmp", "mure-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })

	bin := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	mure := filepath.Join(bin, "mure")
	stub := filepath.Join(bin, "stubagent")
	build(t, root, mure, "./cmd/mure")
	build(t, root, stub, "./test/e2e/stubagent")

	h := &harness{
		t:        t,
		repoRoot: root,
		tmpDir:   tmp,
		mure:     mure,
		stub:     stub,
		tmuxSock: filepath.Join(tmp, "tmux.sock"),
		mureSock: filepath.Join(tmp, "mure.sock"),
	}
	// Env shared by tmux invocations from the test process.
	h.tmuxEnv = append(os.Environ(), "TMUX_TMPDIR="+tmp)
	// Env passed to `mure` subcommands.
	h.mureEnv = append([]string{}, os.Environ()...)
	h.mureEnv = append(h.mureEnv,
		"MURE_SOCKET="+h.mureSock,
		"MURE_SESSION="+tmuxSession,
		"MURE_TMUX_SOCKET="+h.tmuxSock,
		"MURE_AGENT_CMD="+h.stub,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	t.Cleanup(func() {
		_ = h.tmux("kill-server").Run()
	})
	return h
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, _ := os.Getwd()
	dir := wd
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("repo root not found")
	return ""
}

func build(t *testing.T, root, out, pkg string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v: %s", pkg, err, out)
	}
}

func (h *harness) tmux(args ...string) *exec.Cmd {
	full := append([]string{"-S", h.tmuxSock}, args...)
	cmd := exec.Command("tmux", full...)
	cmd.Env = h.tmuxEnv
	return cmd
}

func (h *harness) mureCmd(args ...string) *exec.Cmd {
	cmd := exec.Command(h.mure, args...)
	cmd.Env = h.mureEnv
	return cmd
}

func (h *harness) tmuxOut(args ...string) string {
	h.t.Helper()
	out, err := h.tmux(args...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			h.t.Fatalf("tmux %v: %v: %s", args, err, ee.Stderr)
		}
		h.t.Fatalf("tmux %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func waitFor(t *testing.T, d time.Duration, msg string, pred func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if pred() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s after %v", msg, d)
}

func TestEndToEnd(t *testing.T) {
	h := newHarness(t)

	// Start tmux server with the session.
	if out, err := h.tmux("new-session", "-d", "-s", tmuxSession, "-x", "200", "-y", "50", "sh", "-c", "sleep 600").CombinedOutput(); err != nil {
		t.Fatalf("new-session: %v: %s", err, out)
	}
	// Source the plugin so hooks + sidebar key + pane-border-format are set.
	plugin := filepath.Join(h.repoRoot, "tmux-mure", "tmux-mure.tmux")
	if out, err := h.tmux("run-shell", plugin).CombinedOutput(); err != nil {
		t.Fatalf("source plugin: %v: %s", err, out)
	}

	// mure up (forks daemon).
	out, err := h.mureCmd("up").CombinedOutput()
	if err != nil {
		t.Fatalf("mure up: %v: %s", err, out)
	}
	waitFor(t, 3*time.Second, "daemon socket", func() bool {
		_, err := os.Stat(h.mureSock)
		return err == nil
	})

	// Spawn three agents.
	type spawn struct {
		agentID string
		paneID  string
		panePID int
	}
	var agents []spawn
	for i := 0; i < 3; i++ {
		out, err := h.mureCmd("spawn", "dummy").CombinedOutput()
		if err != nil {
			t.Fatalf("mure spawn: %v: %s", err, out)
		}
		fields := strings.Fields(strings.TrimSpace(string(out)))
		if len(fields) != 2 {
			t.Fatalf("spawn output %q: want 'agentID paneID'", out)
		}
		paneID := fields[1]
		// Get pane PID.
		pidStr := h.tmuxOut("list-panes", "-s", "-t", tmuxSession,
			"-F", "#{pane_id} #{pane_pid}")
		var pid int
		for _, line := range strings.Split(pidStr, "\n") {
			parts := strings.Fields(line)
			if len(parts) == 2 && parts[0] == paneID {
				pid, _ = strconv.Atoi(parts[1])
			}
		}
		// Stub may exec'd through `env`/sh; PID may be wrapper. We send
		// signals to the pane process group; tmux exposes pane_pid which
		// is the head of the pane's process group.
		if pid == 0 {
			t.Fatalf("could not resolve pane PID for %s", paneID)
		}
		agents = append(agents, spawn{agentID: fields[0], paneID: paneID, panePID: pid})
	}

	// Wait until `mure ls --json` shows three agents in `idle`.
	waitFor(t, 3*time.Second, "3 agents idle", func() bool {
		r := readRoster(t, h)
		if len(r.Agents) != 3 {
			return false
		}
		for _, a := range r.Agents {
			if a.Status != sock.StatusIdle {
				return false
			}
		}
		return true
	})

	// Trigger idle→working→idle on each, asserting the @mure-status pane
	// option reflects the transition within 1s.
	for _, a := range agents {
		signalPaneGroup(t, h, a.paneID, syscall.SIGUSR1)
		waitFor(t, 1500*time.Millisecond, a.paneID+" @mure-status=working", func() bool {
			return h.paneStatus(a.paneID) == sock.StatusWorking
		})
		signalPaneGroup(t, h, a.paneID, syscall.SIGUSR2)
		waitFor(t, 1500*time.Millisecond, a.paneID+" @mure-status=idle", func() bool {
			return h.paneStatus(a.paneID) == sock.StatusIdle
		})
	}

	// Toggle sidebar: invoke the plugin script (equivalent to prefix M).
	toggle := filepath.Join(h.repoRoot, "tmux-mure", "scripts", "sidebar-toggle.sh")
	// Invoke via bash directly so script stderr surfaces (tmux run-shell
	// hides it). The script uses plain `tmux` commands which need to find
	// our test server; set TMUX env (socket,pid,session-id) so they do.
	toggleCmd := exec.Command("bash", toggle)
	toggleCmd.Env = append(append([]string{}, h.mureEnv...),
		"TMUX="+h.tmuxSock+",0,0")
	if out, err := toggleCmd.CombinedOutput(); err != nil {
		t.Fatalf("sidebar-toggle: %v: %s", err, out)
	}
	waitFor(t, 2*time.Second, "sidebar pane present", func() bool {
		out := h.tmuxOut("list-panes", "-s", "-t", tmuxSession,
			"-F", "#{pane_id} #{@mure-is-sidebar}")
		for _, line := range strings.Split(out, "\n") {
			if parts := strings.Fields(line); len(parts) == 2 && parts[1] == "1" {
				return true
			}
		}
		return false
	})

	// Capture one pane's status before `down` to verify retention.
	retainedPane := agents[0].paneID
	prevStatus := h.paneStatus(retainedPane)
	if prevStatus == "" {
		t.Fatalf("pre-down @mure-status missing for %s", retainedPane)
	}

	// mure down: socket should disappear; pane options should persist.
	if out, err := h.mureCmd("down").CombinedOutput(); err != nil {
		t.Fatalf("mure down: %v: %s", err, out)
	}
	waitFor(t, 2*time.Second, "socket removed", func() bool {
		_, err := os.Stat(h.mureSock)
		return os.IsNotExist(err)
	})
	if got := h.paneStatus(retainedPane); got != prevStatus {
		t.Fatalf("pane option lost after down: %s @mure-status=%q want %q",
			retainedPane, got, prevStatus)
	}
}

func TestEndToEndSubagentsWindow(t *testing.T) {
	h := newHarness(t)

	if out, err := h.tmux("new-session", "-d", "-s", tmuxSession, "-x", "200", "-y", "50", "sh", "-c", "sleep 600").CombinedOutput(); err != nil {
		t.Fatalf("new-session: %v: %s", err, out)
	}
	plugin := filepath.Join(h.repoRoot, "tmux-mure", "tmux-mure.tmux")
	if out, err := h.tmux("run-shell", plugin).CombinedOutput(); err != nil {
		t.Fatalf("source plugin: %v: %s", err, out)
	}

	// Daemon up.
	if out, err := h.mureCmd("up").CombinedOutput(); err != nil {
		t.Fatalf("mure up: %v: %s", err, out)
	}
	waitFor(t, 3*time.Second, "daemon socket", func() bool {
		_, err := os.Stat(h.mureSock)
		return err == nil
	})

	origWindow := h.tmuxOut("display-message", "-p", "-t", tmuxSession, "#{window_id}")

	// Two spawns using the default (subagents-window).
	var panes []string
	for i := 0; i < 2; i++ {
		out, err := h.mureCmd("spawn", "dummy").CombinedOutput()
		if err != nil {
			t.Fatalf("mure spawn: %v: %s", err, out)
		}
		fields := strings.Fields(strings.TrimSpace(string(out)))
		if len(fields) != 2 {
			t.Fatalf("spawn output %q: want 'agentID paneID'", out)
		}
		panes = append(panes, fields[1])
	}

	// Two windows in the session.
	wins := strings.Split(h.tmuxOut("list-windows", "-t", tmuxSession, "-F", "#{window_id} #{window_name}"), "\n")
	if len(wins) != 2 {
		t.Fatalf("want 2 windows, got %d: %v", len(wins), wins)
	}
	var subWin string
	for _, line := range wins {
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[1] == "subagents" {
			subWin = parts[0]
		}
	}
	if subWin == "" {
		t.Fatalf("no subagents window in %v", wins)
	}

	// Marker option set.
	marker := h.tmuxOut("show-options", "-wv", "-t", subWin, "@mure-subagents-window")
	if marker != "1" {
		t.Fatalf("@mure-subagents-window = %q, want 1", marker)
	}

	// 2 panes in subagents window.
	subPanes := strings.Split(h.tmuxOut("list-panes", "-t", subWin, "-F", "#{pane_id}"), "\n")
	if len(subPanes) != 2 {
		t.Fatalf("want 2 panes in subagents window, got %d: %v", len(subPanes), subPanes)
	}

	// Original window still active.
	active := h.tmuxOut("display-message", "-p", "-t", tmuxSession, "#{window_id}")
	if active != origWindow {
		t.Fatalf("active window = %s, want original %s", active, origWindow)
	}

	// Worker-spawn: simulate `mure spawn` invoked from inside a subagent
	// pane by setting TMUX_PANE to that pane's id. This exercises the
	// resolveSessionID path that uses TMUX_PANE.
	workerCmd := exec.Command(h.mure, "spawn", "dummy")
	workerCmd.Env = append(append([]string{}, h.mureEnv...), "TMUX_PANE="+panes[0])
	if out, err := workerCmd.CombinedOutput(); err != nil {
		t.Fatalf("worker mure spawn: %v: %s", err, out)
	}
	waitFor(t, 3*time.Second, "3 panes in subagents window", func() bool {
		out := h.tmuxOut("list-panes", "-t", subWin, "-F", "#{pane_id}")
		return len(strings.Split(out, "\n")) == 3
	})
	wins2 := strings.Split(h.tmuxOut("list-windows", "-t", tmuxSession, "-F", "#{window_id}"), "\n")
	if len(wins2) != 2 {
		t.Fatalf("after worker spawn want 2 windows, got %d: %v", len(wins2), wins2)
	}

	// No-regression: switch to right-of-active and spawn lands in original window.
	if out, err := h.tmux("set-option", "-g", "@mure-spawn-target", "right-of-active").CombinedOutput(); err != nil {
		t.Fatalf("set-option: %v: %s", err, out)
	}
	origPanesBefore := len(strings.Split(h.tmuxOut("list-panes", "-t", origWindow, "-F", "#{pane_id}"), "\n"))
	if out, err := h.mureCmd("spawn", "dummy").CombinedOutput(); err != nil {
		t.Fatalf("mure spawn (right-of-active): %v: %s", err, out)
	}
	wins3 := strings.Split(h.tmuxOut("list-windows", "-t", tmuxSession, "-F", "#{window_id}"), "\n")
	if len(wins3) != 2 {
		t.Fatalf("right-of-active: want 2 windows, got %d: %v", len(wins3), wins3)
	}
	origPanesAfter := len(strings.Split(h.tmuxOut("list-panes", "-t", origWindow, "-F", "#{pane_id}"), "\n"))
	if origPanesAfter != origPanesBefore+1 {
		t.Fatalf("original window pane count: before=%d after=%d", origPanesBefore, origPanesAfter)
	}

	_ = h.mureCmd("down").Run()
}

func readRoster(t *testing.T, h *harness) sock.Roster {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, h.mure, "ls", "--json")
	cmd.Env = h.mureEnv
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			t.Logf("ls --json stderr: %s", ee.Stderr)
		}
		t.Fatalf("mure ls --json: %v", err)
	}
	var r sock.Roster
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatalf("decode roster: %v: %s", err, out)
	}
	return r
}

func (h *harness) paneStatus(paneID string) string {
	out, err := h.tmux("show-options", "-pv", "-t", paneID, "@mure-status").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func signalPaneGroup(t *testing.T, h *harness, paneID string, sig syscall.Signal) {
	t.Helper()
	// Resolve pane child PID (the stub agent process). The pane_pid is
	// often a shell wrapper; the agent runs under it.
	out := h.tmuxOut("list-panes", "-s", "-t", tmuxSession,
		"-F", "#{pane_id} #{pane_pid}")
	var ppid int
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[0] == paneID {
			ppid, _ = strconv.Atoi(parts[1])
		}
	}
	if ppid == 0 {
		t.Fatalf("no pane_pid for %s", paneID)
	}
	// Signal the whole process group so we hit the stub regardless of
	// wrapper depth (sh/env/stub).
	if err := syscall.Kill(-ppid, sig); err != nil {
		// Fall back to per-PID signal.
		_ = syscall.Kill(ppid, sig)
		// Then walk children.
		if out, err := exec.Command("pgrep", "-P", strconv.Itoa(ppid)).Output(); err == nil {
			for _, line := range strings.Fields(string(out)) {
				if pid, _ := strconv.Atoi(line); pid > 0 {
					_ = syscall.Kill(pid, sig)
				}
			}
		}
	}
}
