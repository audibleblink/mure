package daemon

import (
	"sync"
	"testing"
	"time"

	"github.com/audibleblink/mure/internal/sock"
)

func helloFor(id, pane string) sock.Hello {
	return sock.Hello{V: sock.ProtocolVersion, Event: "hello", Role: sock.RoleAgent, AgentID: id, PaneID: pane}
}

func TestRosterConcurrentUpsertsSerialized(t *testing.T) {
	r := NewRoster()
	defer r.Close()

	const n = 200
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			r.UpsertFromHello(helloFor("a", "%1"))
			r.ApplyStatus(sock.Status{V: 1, Event: "status", AgentID: "a", Status: sock.StatusWorking, TS: int64(i)})
		}(i)
	}
	wg.Wait()
	snap := r.Snapshot()
	if len(snap.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(snap.Agents))
	}
	if snap.Agents[0].Status != sock.StatusWorking {
		t.Fatalf("expected working, got %q", snap.Agents[0].Status)
	}
}

func TestRosterSubscriberReceivesSequence(t *testing.T) {
	r := NewRoster()
	defer r.Close()
	ch, cancel := r.Subscribe()
	defer cancel()

	r.UpsertFromHello(helloFor("a", "%1"))
	r.ApplyStatus(sock.Status{V: 1, Event: "status", AgentID: "a", Status: sock.StatusWorking, TS: 1})
	r.ApplyStatus(sock.Status{V: 1, Event: "status", AgentID: "a", Status: sock.StatusIdle, TS: 2})

	wantStatuses := []string{sock.StatusIdle, sock.StatusWorking, sock.StatusIdle}
	for i, want := range wantStatuses {
		select {
		case upd := <-ch:
			if upd.Agent.Status != want {
				t.Fatalf("step %d: got %q want %q", i, upd.Agent.Status, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("step %d: timeout", i)
		}
	}
}

func TestRosterOverflowClosesSubscriber(t *testing.T) {
	r := NewRoster()
	defer r.Close()
	ch, _ := r.Subscribe()
	// Slow consumer: never drain. Push subscriberBufSize+10 updates.
	for i := 0; i < subscriberBufSize+10; i++ {
		r.ApplyStatus(sock.Status{V: 1, Event: "status", AgentID: "a", Status: sock.StatusWorking, TS: int64(i)})
	}
	// Drain whatever's in the buffer; channel must eventually be closed.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Fatal("subscriber channel was not closed after overflow")
}

func TestRosterRemove(t *testing.T) {
	r := NewRoster()
	defer r.Close()
	r.UpsertFromHello(helloFor("a", "%1"))
	r.Remove("a")
	if len(r.Snapshot().Agents) != 0 {
		t.Fatal("expected agent removed")
	}
}
