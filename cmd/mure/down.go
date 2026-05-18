package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/audibleblink/mure/internal/sock"
)

func cmdDown(ctx context.Context, _ []string, stdout, stderr *os.File) int {
	path := resolveSocket()
	if path == "" {
		fmt.Fprintln(stderr, "mure down: MURE_SOCKET not set")
		return 1
	}
	conn, br, err := dialHello(path, sock.RoleCLI, time.Second)
	if err != nil {
		fmt.Fprintf(stderr, "mure down: %v\n", err)
		return 1
	}
	defer conn.Close()
	// Drain initial roster response.
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	_, _ = sock.ReadFrame(br, sock.MaxFrameSize)
	if err := sock.WriteFrame(conn, sock.Envelope{V: sock.ProtocolVersion, Event: "shutdown"}); err != nil {
		fmt.Fprintf(stderr, "mure down: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "shutdown requested")
	_ = ctx
	return 0
}
