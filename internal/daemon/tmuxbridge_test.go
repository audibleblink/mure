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

// TestBridgePrunesAgentsWithDeadPanes drives the bridge through one
// reconcile call and verifies that an agent whose pane is missing from
// the list-panes reply is removed.
func TestBridgePrunesAgentsWithDeadPanes(t *testing.T) {
	roster := NewRoster()
	defer roster.Close()
	c := tmuxctl.NewScriptedFake()
	defer c.Close()

	roster.UpsertFromHello(sock.Hello{V: 1, Event: "hello", Role: sock.RoleAgent, AgentID: "alive", PaneID: "%41"})
	roster.UpsertFromHello(sock.Hello{V: 1, Event: "hello", Role: sock.RoleAgent, AgentID: "dead", PaneID: "%42"})

	// Reply: only %41 is alive. Format includes the bridge's "p=" prefix.
	c.EnqueueReply("p=%41\n", nil)

	b := NewBridge(c, roster, "test", nil)
	b.reconcile(context.Background())

	waitFor(t, time.Second, "dead pruned, alive kept", func() bool {
		var sawAlive, sawDead bool
		for _, a := range roster.Snapshot().Agents {
			if a.ID == "alive" {
				sawAlive = true
			}
			if a.ID == "dead" {
				sawDead = true
			}
		}
		return sawAlive && !sawDead
	})
}
