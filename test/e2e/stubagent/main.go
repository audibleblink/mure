// Stub agent for e2e tests: speaks the mure socket protocol, mimicking
// what pi-mure does at runtime.
//
// Lifecycle:
//   - Connects to $MURE_SOCKET, sends hello{role=agent} then an initial
//     status=idle frame.
//   - Reacts to signals:
//     SIGUSR1 -> status=working
//     SIGUSR2 -> status=idle
//     SIGTERM -> send bye and exit 0
package main

import (
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/audibleblink/mure/internal/sock"
)

func main() {
	agentID := os.Getenv("MURE_AGENT_ID")
	sockPath := os.Getenv("MURE_SOCKET")
	if agentID == "" || sockPath == "" {
		os.Exit(2)
	}
	paneID := os.Getenv("TMUX_PANE")

	var conn net.Conn
	for i := 0; i < 50; i++ {
		c, err := net.Dial("unix", sockPath)
		if err == nil {
			conn = c
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if conn == nil {
		os.Exit(3)
	}
	defer conn.Close()

	now := func() int64 { return time.Now().UnixMilli() }
	_ = sock.WriteFrame(conn, sock.Hello{
		V: sock.ProtocolVersion, Event: "hello", Role: sock.RoleAgent,
		AgentID: agentID, PaneID: paneID, PID: os.Getpid(),
		PiVersion: "stub", TS: now(),
	})
	_ = sock.WriteFrame(conn, sock.Status{
		V: sock.ProtocolVersion, Event: "status",
		AgentID: agentID, Status: sock.StatusIdle, TS: now(),
	})

	sigs := make(chan os.Signal, 8)
	signal.Notify(sigs, syscall.SIGUSR1, syscall.SIGUSR2, syscall.SIGTERM, syscall.SIGINT)
	for s := range sigs {
		switch s {
		case syscall.SIGUSR1:
			_ = sock.WriteFrame(conn, sock.Status{
				V: sock.ProtocolVersion, Event: "status",
				AgentID: agentID, Status: sock.StatusWorking, TS: now(),
			})
		case syscall.SIGUSR2:
			_ = sock.WriteFrame(conn, sock.Status{
				V: sock.ProtocolVersion, Event: "status",
				AgentID: agentID, Status: sock.StatusIdle, TS: now(),
			})
		case syscall.SIGTERM, syscall.SIGINT:
			_ = sock.WriteFrame(conn, sock.Bye{
				V: sock.ProtocolVersion, Event: "bye",
				AgentID: agentID, TS: now(),
			})
			return
		}
	}
}
