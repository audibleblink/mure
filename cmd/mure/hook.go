package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/audibleblink/mure/internal/sock"
)

// cmdHook is `mure _hook <event> <args...>` (PRD §12.2). Best-effort:
// connect, send one frame, exit. Failures are silent (exit 0) so noisy
// tmux hooks don't spam terminals.
func cmdHook(ctx context.Context, argv []string, _, stderr *os.File) int {
	if len(argv) < 1 {
		fmt.Fprintln(stderr, "usage: mure _hook <event> [args...]")
		return 2
	}
	path := resolveSocket()
	if path == "" {
		return 0
	}
	conn, _, err := dialHello(path, sock.RoleHook, 500*time.Millisecond)
	if err != nil {
		return 0
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(500 * time.Millisecond))

	now := time.Now().UnixMilli()
	event := argv[0]
	rest := argv[1:]
	var frame any
	switch event {
	case "focus":
		if len(rest) < 1 {
			return 2
		}
		focused := true
		client := ""
		if len(rest) >= 2 {
			client = rest[1]
		}
		frame = sock.Focus{V: 1, Event: "focus", PaneID: rest[0], Client: client, Focused: &focused, TS: now}
	case "pane_died":
		if len(rest) < 1 {
			return 2
		}
		frame = sock.PaneDied{V: 1, Event: "pane_died", PaneID: rest[0]}
	case "session_closed":
		if len(rest) < 1 {
			return 2
		}
		frame = sock.SessionClosed{V: 1, Event: "session_closed", Session: rest[0]}
	default:
		fmt.Fprintf(stderr, "mure _hook: unknown event %q\n", event)
		return 2
	}
	_ = sock.WriteFrame(conn, frame)
	_ = ctx
	return 0
}
