// Package daemon implements the mure daemon: roster state machine, Unix
// socket server, peer-auth, and timers (debounce/coalesce). All tmux I/O
// lives in Phase 5's tmuxbridge; this package is independent of tmux.
package daemon

import (
	"sync"
	"time"

	"github.com/audibleblink/mure/internal/sock"
)

// subscriberBufSize bounds each sidebar/cli subscriber's update queue.
// On overflow the channel is closed (PRD §9 backpressure).
const subscriberBufSize = 64

// agentState is the in-memory record for one agent.
type agentState struct {
	ID              string
	PaneID          string
	Role            string
	PID             int
	PiVersion       string
	Status          string
	Task            string
	LastTurnEndedAt int64
	Result          string
}

func (a *agentState) snapshot() sock.AgentSnapshot {
	return sock.AgentSnapshot{
		ID:              a.ID,
		Status:          a.Status,
		Role:            a.Role,
		Task:            a.Task,
		Pane:            a.PaneID,
		LastTurnEndedAt: a.LastTurnEndedAt,
		Result:          a.Result,
	}
}

// Roster is the canonical agent registry. All mutations flow through a single
// goroutine via the request channel (PRD §6.2).
type Roster struct {
	reqs   chan func(*rosterCore)
	quit   chan struct{}
	doneWG sync.WaitGroup
}

type rosterCore struct {
	agents      map[string]*agentState
	subscribers map[*subscriber]struct{}
	launchDir   string
}

type subscriber struct {
	ch     chan sock.AgentUpdate
	closed bool
}

// NewRoster constructs and starts a Roster. Call Close to stop its goroutine.
func NewRoster() *Roster {
	r := &Roster{
		reqs: make(chan func(*rosterCore), 64),
		quit: make(chan struct{}),
	}
	core := &rosterCore{
		agents:      make(map[string]*agentState),
		subscribers: make(map[*subscriber]struct{}),
	}
	r.doneWG.Add(1)
	go r.run(core)
	return r
}

func (r *Roster) run(core *rosterCore) {
	defer r.doneWG.Done()
	for {
		select {
		case <-r.quit:
			for s := range core.subscribers {
				if !s.closed {
					close(s.ch)
					s.closed = true
				}
			}
			return
		case fn := <-r.reqs:
			fn(core)
		}
	}
}

// Close stops the goroutine and closes all subscriber channels.
func (r *Roster) Close() {
	select {
	case <-r.quit:
		return
	default:
	}
	close(r.quit)
	r.doneWG.Wait()
}

func (r *Roster) submit(fn func(*rosterCore)) {
	select {
	case <-r.quit:
	case r.reqs <- fn:
	}
}

func (r *Roster) submitWait(fn func(*rosterCore)) {
	done := make(chan struct{})
	r.submit(func(c *rosterCore) {
		fn(c)
		close(done)
	})
	<-done
}

// UpsertFromHello records a new or returning agent from a hello frame.
func (r *Roster) UpsertFromHello(h sock.Hello) {
	r.submit(func(c *rosterCore) {
		a, ok := c.agents[h.AgentID]
		if !ok {
			a = &agentState{ID: h.AgentID, Status: sock.StatusIdle}
			c.agents[h.AgentID] = a
		}
		a.PaneID = h.PaneID
		a.PID = h.PID
		a.PiVersion = h.PiVersion
		if h.AgentRole != "" {
			a.Role = h.AgentRole
		}
		broadcast(c, a)
	})
}

// ApplyStatus folds a status frame into the roster.
func (r *Roster) ApplyStatus(s sock.Status) {
	r.submit(func(c *rosterCore) {
		a, ok := c.agents[s.AgentID]
		if !ok {
			a = &agentState{ID: s.AgentID}
			c.agents[s.AgentID] = a
		}
		a.Status = s.Status
		a.Task = s.Task
		if s.Status == sock.StatusIdle {
			a.LastTurnEndedAt = s.TS
		}
		broadcast(c, a)
	})
}

// ApplyResult records an agent's final-answer text and broadcasts.
func (r *Roster) ApplyResult(res sock.Result) {
	r.submit(func(c *rosterCore) {
		a, ok := c.agents[res.AgentID]
		if !ok {
			a = &agentState{ID: res.AgentID}
			c.agents[res.AgentID] = a
		}
		a.Result = res.Text
		broadcast(c, a)
	})
}

// MarkDisconnected schedules a transition to "disconnected" after `after`.
// If the agent has been removed or transitioned in the meantime, no-op.
func (r *Roster) MarkDisconnected(agentID string, after time.Duration) {
	time.AfterFunc(after, func() {
		r.submit(func(c *rosterCore) {
			a, ok := c.agents[agentID]
			if !ok {
				return
			}
			a.Status = sock.StatusDisconnected
			broadcast(c, a)
		})
	})
}

// MarkErrored transitions an agent to "errored".
func (r *Roster) MarkErrored(agentID string) {
	r.submit(func(c *rosterCore) {
		a, ok := c.agents[agentID]
		if !ok {
			a = &agentState{ID: agentID}
			c.agents[agentID] = a
		}
		a.Status = sock.StatusErrored
		broadcast(c, a)
	})
}

// Remove deletes an agent and broadcasts a deletion update to subscribers.
func (r *Roster) Remove(agentID string) {
	r.submit(func(c *rosterCore) {
		if _, ok := c.agents[agentID]; !ok {
			return
		}
		delete(c.agents, agentID)
		upd := sock.AgentUpdate{
			V:       sock.ProtocolVersion,
			Event:   "agent_update",
			Agent:   sock.AgentSnapshot{ID: agentID},
			Deleted: true,
		}
		for s := range c.subscribers {
			if s.closed {
				delete(c.subscribers, s)
				continue
			}
			select {
			case s.ch <- upd:
			default:
				close(s.ch)
				s.closed = true
				delete(c.subscribers, s)
			}
		}
	})
}

// Snapshot returns a sock.Roster of the current state.
func (r *Roster) Snapshot() sock.Roster {
	var out sock.Roster
	r.submitWait(func(c *rosterCore) {
		out.V = sock.ProtocolVersion
		out.Event = "roster"
		out.LaunchDir = c.launchDir
		out.Agents = make([]sock.AgentSnapshot, 0, len(c.agents))
		for _, a := range c.agents {
			out.Agents = append(out.Agents, a.snapshot())
		}
	})
	return out
}

// SetLaunchDir records the directory under which `mure up` started.
// It is included in roster snapshots sent to sidebars.
func (r *Roster) SetLaunchDir(dir string) {
	r.submitWait(func(c *rosterCore) { c.launchDir = dir })
}

// Subscribe registers a subscriber. The returned channel receives agent_update
// frames; if the subscriber lags more than subscriberBufSize, the channel is
// closed. The cancel func unsubscribes idempotently.
func (r *Roster) Subscribe() (<-chan sock.AgentUpdate, func()) {
	s := &subscriber{ch: make(chan sock.AgentUpdate, subscriberBufSize)}
	r.submitWait(func(c *rosterCore) {
		c.subscribers[s] = struct{}{}
	})
	cancel := func() {
		r.submit(func(c *rosterCore) {
			if _, ok := c.subscribers[s]; !ok {
				return
			}
			delete(c.subscribers, s)
			if !s.closed {
				close(s.ch)
				s.closed = true
			}
		})
	}
	return s.ch, cancel
}

// broadcast emits an agent_update for a to all subscribers; slow subscribers
// (full buffer) are dropped.
func broadcast(c *rosterCore, a *agentState) {
	upd := sock.AgentUpdate{V: sock.ProtocolVersion, Event: "agent_update", Agent: a.snapshot()}
	for s := range c.subscribers {
		if s.closed {
			delete(c.subscribers, s)
			continue
		}
		select {
		case s.ch <- upd:
		default:
			// Overflow: drop subscriber.
			close(s.ch)
			s.closed = true
			delete(c.subscribers, s)
		}
	}
}
