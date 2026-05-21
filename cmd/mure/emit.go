package main

// `mure emit` is the canonical NDJSON producer used by harness hook scripts.
// Each invocation opens a transient ("oneshot") agent connection to the
// daemon, sends one status (sock.Status) or result (sock.Result) frame, then
// closes. Wire-format decision: sock.Status already has an optional Tool
// field, so no rename was needed; we did add one additive field —
// sock.Hello.Oneshot — so the daemon does not treat the post-emit close as
// the owning agent's death.

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/audibleblink/mure/internal/sock"
)

func cmdEmit(ctx context.Context, argv []string, stdout, stderr *os.File) int {
	if len(argv) < 1 {
		fmt.Fprintln(stderr, "usage: mure emit status <working|blocked|idle> [--tool <name>]\n       mure emit result -")
		return 2
	}
	switch argv[0] {
	case "status":
		return emitStatus(ctx, argv[1:], stderr)
	case "result":
		return emitResult(ctx, argv[1:], os.Stdin, stderr)
	default:
		fmt.Fprintf(stderr, "mure emit: unknown subcommand %q\n", argv[0])
		return 2
	}
}

func emitStatus(ctx context.Context, argv []string, stderr *os.File) int {
	// Allow --tool to appear either before or after the positional status.
	var positional []string
	var tool string
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "--tool":
			if i+1 >= len(argv) {
				fmt.Fprintln(stderr, "mure emit: --tool requires a value")
				return 2
			}
			tool = argv[i+1]
			i++
		case strings.HasPrefix(a, "--tool="):
			tool = strings.TrimPrefix(a, "--tool=")
		default:
			positional = append(positional, a)
		}
	}
	if len(positional) != 1 {
		fmt.Fprintln(stderr, "usage: mure emit status <working|blocked|idle> [--tool <name>]")
		return 2
	}
	status := positional[0]
	if !sock.ValidStatus(status) {
		fmt.Fprintf(stderr, "mure emit: invalid status %q (want working|blocked|idle)\n", status)
		return 2
	}
	sockPath := os.Getenv("MURE_SOCKET")
	if sockPath == "" {
		fmt.Fprintln(stderr, "mure emit: MURE_SOCKET not set")
		return 1
	}
	agentID, err := requireAgentID(stderr)
	if err != nil {
		return 1
	}
	frame := sock.Status{
		V:       sock.ProtocolVersion,
		Event:   "status",
		AgentID: agentID,
		Status:  status,
		Tool:    tool,
		TS:      time.Now().UnixMilli(),
	}
	return sendOneshot(ctx, sockPath, agentID, frame, stderr)
}

func emitResult(ctx context.Context, argv []string, stdin io.Reader, stderr *os.File) int {
	if len(argv) != 1 || argv[0] != "-" {
		fmt.Fprintln(stderr, "usage: mure emit result -")
		return 2
	}
	sockPath := os.Getenv("MURE_SOCKET")
	if sockPath == "" {
		fmt.Fprintln(stderr, "mure emit: MURE_SOCKET not set")
		return 1
	}
	agentID, err := requireAgentID(stderr)
	if err != nil {
		return 1
	}
	body, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "mure emit: read stdin: %v\n", err)
		return 1
	}
	frame := sock.Result{
		V:       sock.ProtocolVersion,
		Event:   "result",
		AgentID: agentID,
		Text:    string(body),
		TS:      time.Now().UnixMilli(),
	}
	return sendOneshot(ctx, sockPath, agentID, frame, stderr)
}

func requireAgentID(stderr *os.File) (string, error) {
	id := os.Getenv("MURE_AGENT_ID")
	if id == "" {
		fmt.Fprintln(stderr, "mure emit: MURE_AGENT_ID not set")
		return "", fmt.Errorf("missing agent id")
	}
	return id, nil
}

func emitPaneID(ctx context.Context) string {
	if p := os.Getenv("MURE_PANE_ID"); p != "" {
		return p
	}
	if p := os.Getenv("TMUX_PANE"); p != "" {
		return p
	}
	if os.Getenv("TMUX") == "" {
		return ""
	}
	cmd := exec.CommandContext(ctx, "tmux", "display-message", "-p", "#{pane_id}")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func sendOneshot(ctx context.Context, sockPath, agentID string, frame any, stderr *os.File) int {
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.DialContext(ctx, "unix", sockPath)
	if err != nil {
		fmt.Fprintf(stderr, "mure emit: dial: %v\n", err)
		return 1
	}
	defer conn.Close()
	hello := sock.Hello{
		V:       sock.ProtocolVersion,
		Event:   "hello",
		Role:    sock.RoleAgent,
		AgentID: agentID,
		PaneID:  emitPaneID(ctx),
		Oneshot: true,
		TS:      time.Now().UnixMilli(),
	}
	if err := sock.WriteFrame(conn, hello); err != nil {
		fmt.Fprintf(stderr, "mure emit: hello: %v\n", err)
		return 1
	}
	if err := sock.WriteFrame(conn, frame); err != nil {
		fmt.Fprintf(stderr, "mure emit: write: %v\n", err)
		return 1
	}
	return 0
}
