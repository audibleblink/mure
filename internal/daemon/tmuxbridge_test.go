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
	reader := tmuxctl.NewScriptedFake()
	defer reader.Close()
	writer := tmuxctl.NewScriptedFake()
	defer writer.Close()
	deb := NewDebouncer(50*time.Millisecond,
		func(id string) { roster.Remove(id) },
		func(id string) { roster.MarkDisconnected(id, 0) })
	defer deb.Stop()

	roster.UpsertFromHello(sock.Hello{V: 1, Event: "hello", Role: sock.RoleAgent, AgentID: "a1", PaneID: "%41"})

	b := NewBridge(reader, writer, roster, deb, "test")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go b.Run(ctx)

	// Give bridge a moment to subscribe before we emit the event.
	time.Sleep(20 * time.Millisecond)

	reader.EmitEvent(tmuxctl.Event{Kind: tmuxctl.EventPaneDied, PaneID: "%41"})

	waitFor(t, time.Second, "a1 removed", func() bool {
		for _, a := range roster.Snapshot().Agents {
			if a.ID == "a1" {
				return false
			}
		}
		return true
	})
}
