package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/audibleblink/mure/internal/sock"
)

// cmdWait blocks until <agent> has a recorded result (or terminal state),
// then prints the result text to stdout. Exit 0 on result, 1 on errored.
func cmdWait(ctx context.Context, argv []string, stdout, stderr *os.File) int {
	if len(argv) < 1 {
		fmt.Fprintln(stderr, "usage: mure wait <agent>")
		return 2
	}
	agentID := argv[0]
	path := resolveSocket()
	if path == "" {
		fmt.Fprintln(stderr, "mure wait: MURE_SOCKET not set")
		return 1
	}
	conn, br, err := dialHello(path, sock.RoleCLI, time.Second)
	if err != nil {
		fmt.Fprintf(stderr, "mure wait: %v\n", err)
		return 1
	}
	defer conn.Close()
	// Drain initial snapshot.
	if _, err := sock.ReadFrame(br, sock.MaxFrameSize); err != nil {
		fmt.Fprintf(stderr, "mure wait: %v\n", err)
		return 1
	}
	if err := sock.WriteFrame(conn, sock.Wait{V: sock.ProtocolVersion, Event: "wait", AgentID: agentID}); err != nil {
		fmt.Fprintf(stderr, "mure wait: %v\n", err)
		return 1
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetReadDeadline(dl)
	}
	line, err := sock.ReadFrame(br, sock.MaxFrameSize)
	if err != nil {
		fmt.Fprintf(stderr, "mure wait: %v\n", err)
		return 1
	}
	var upd sock.AgentUpdate
	if err := json.Unmarshal(line, &upd); err != nil {
		fmt.Fprintf(stderr, "mure wait: decode: %v\n", err)
		return 1
	}
	if upd.Agent.Result != "" {
		fmt.Fprintln(stdout, upd.Agent.Result)
		return 0
	}
	fmt.Fprintf(stderr, "mure wait: agent %s ended in status %q with no result\n", upd.Agent.ID, upd.Agent.Status)
	return 1
}
