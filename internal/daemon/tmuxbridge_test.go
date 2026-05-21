package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/audibleblink/mure/internal/sock"
	"github.com/audibleblink/mure/internal/tmuxctl"
)

func waitFor(t *testing.T, d time.Duration, msg string, pred func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if pred() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for: %s", msg)
}

func TestBridgePaneDiedRemovesAgent(t *testing.T) {
	roster := NewRoster()
	defer roster.Close()
	c := tmuxctl.NewScriptedFake()
	defer c.Close()

	roster.UpsertFromHello(sock.Hello{V: 1, Event: "hello", Role: sock.RoleAgent, AgentID: "a1", PaneID: "%41"})

	b := NewBridge(c, roster, "test")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go b.Run(ctx)

	time.Sleep(20 * time.Millisecond)
	c.EmitEvent(tmuxctl.Event{Kind: tmuxctl.EventPaneDied, PaneID: "%41"})

	waitFor(t, time.Second, "a1 removed", func() bool {
		for _, a := range roster.Snapshot().Agents {
			if a.ID == "a1" {
				return false
			}
		}
		return true
	})
}
