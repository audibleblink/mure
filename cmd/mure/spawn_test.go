package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/audibleblink/mure/internal/daemon"
	"github.com/audibleblink/mure/internal/harnesses"
	"github.com/audibleblink/mure/internal/sock"
)

func TestResolveHarness_Precedence(t *testing.T) {
	noTmux := func(args ...string) (string, error) { return "", errors.New("no tmux") }
	yesSession := func(args ...string) (string, error) {
		if len(args) >= 2 && args[0] == "show-option" && args[1] == "-qv" {
			return "from-session", nil
		}
		return "", errors.New("nope")
	}
	yesGlobal := func(args ...string) (string, error) {
		if len(args) >= 2 && args[0] == "show-option" && args[1] == "-gqv" {
			return "from-global", nil
		}
		return "", errors.New("nope")
	}

	cases := []struct {
		name string
		flag string
		env  string
		run  tmuxRunner
		want string
		err  bool
	}{
		{"flag wins", "fl", "ev", yesSession, "fl", false},
		{"env when no flag", "", "ev", yesSession, "ev", false},
		{"session when no env", "", "", yesSession, "from-session", false},
		{"global when no session", "", "", yesGlobal, "from-global", false},
		{"none → error", "", "", noTmux, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := resolveHarness(c.flag, c.env, c.run)
			if c.err {
				if err == nil {
					t.Fatal("want error")
				}
				msg := err.Error()
				for _, slot := range []string{"--harness", "MURE_HARNESS", "session", "global"} {
					if !strings.Contains(msg, slot) {
						t.Errorf("error %q missing slot %q", msg, slot)
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

// TestSpawn_SendsRegisterPane drives cmdSpawn against an in-process daemon
// and asserts the pane→harness binding is recorded.
func TestSpawn_SendsRegisterPane(t *testing.T) {
	// Inject a fake harness manifest via fstest.MapFS.
	mfs := fstest.MapFS{
		"fake/manifest.toml": &fstest.MapFile{Data: []byte(`
name = "fake"
command = "true"
task_arg = "positional"
[capabilities]
spawn = true
status = false
result = false
`)},
	}
	harnesses.SetFSForTesting(mfs)
	t.Cleanup(func() { harnesses.SetFSForTesting(nil) })

	// Stub tmux: emulate split-window and set-option success.
	tmuxBin := writeStubTmux(t)
	t.Setenv("PATH", filepath.Dir(tmuxBin)+":"+os.Getenv("PATH"))
	// Force "subagents-window" path to short-circuit to a known plan.
	t.Setenv("TMUX_PANE", "%99")

	sockPath, roster := startInProcessDaemon(t)
	t.Setenv("MURE_SOCKET", sockPath)
	t.Setenv("MURE_SESSION", "test-sess")
	// Avoid resolveHarness using tmux global option lookup: pass via flag.

	exit, out, errs := captureRun(t, []string{"spawn", "--harness", "fake", "myrole", "do a thing"})
	if exit != 0 {
		t.Fatalf("exit=%d\nstdout=%s\nstderr=%s", exit, out, errs)
	}
	// Stdout should be: "<agentID> <paneID>\n"
	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) != 2 {
		t.Fatalf("unexpected stdout: %q", out)
	}
	agentID, paneID := fields[0], fields[1]

	// Poll roster.PaneBinding until daemon registers.
	deadline := time.Now().Add(2 * time.Second)
	var harness string
	var statusCap, resultCap, ok bool
	for time.Now().Before(deadline) {
		harness, statusCap, resultCap, ok = roster.PaneBinding(paneID)
		if ok {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ok {
		t.Fatalf("daemon never received register_pane for paneID=%s", paneID)
	}
	if harness != "fake" {
		t.Errorf("harness=%q want fake", harness)
	}
	if statusCap || resultCap {
		t.Errorf("expected capabilities false; got status=%v result=%v", statusCap, resultCap)
	}

	// Agent should be marked degraded in the snapshot.
	snap := roster.Snapshot()
	var found bool
	for _, a := range snap.Agents {
		if a.ID == agentID {
			found = true
			if !a.Degraded {
				t.Errorf("agent %s not marked degraded", agentID)
			}
		}
	}
	if !found {
		t.Errorf("agent %s missing from roster", agentID)
	}
}

// writeStubTmux drops a small "tmux" script into a temp dir so PATH lookups
// resolve to a deterministic stub. The stub handles the subset of tmux verbs
// cmdSpawn invokes.
func writeStubTmux(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tmux")
	body := `#!/usr/bin/env bash
# Args after possible -S <sock> prefix.
if [ "$1" = "-S" ]; then shift 2; fi
case "$1" in
  show-option)
    # show-option [-p|-g|-w] [-qv|-gqv] <name> [val]
    # Used for @mure-spawn-target lookup; emit empty.
    exit 0
    ;;
  display-message)
    # display-message -p [-t pane] '#{session_id}' or '#{window_id}'
    last="${@: -1}"
    case "$last" in
      *session_id*) echo '$1' ;;
      *window_id*)  echo '@7' ;;
      *)            echo '' ;;
    esac
    ;;
  list-windows)
    echo ''
    ;;
  new-window|split-window)
    # Print pane id for -P -F #{pane_id}.
    echo '%42'
    ;;
  set-option)
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// suppress unused-import warnings if any helper is omitted later.
var _ = json.Unmarshal
var _ = io.EOF
var _ = bufio.NewReader
var _ = net.Dial
var _ = context.Background
var _ = daemon.Check
var _ = sock.ProtocolVersion
