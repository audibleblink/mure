package daemon

import (
	"sync"
	"time"
)

// DefaultDebounceWindow is the EPIPE-vs-pane-died window (PRD §6.3).
const DefaultDebounceWindow = 1 * time.Second

// Debouncer decides whether an agent's connection loss was a "pane died"
// (errored) or merely a graceful/abrupt disconnect.
//
// Usage: when an EPIPE/EOF is observed for an agent, call OnEPIPE(agentID).
// If pane_died arrives for the same agent within the window, OnPaneDied
// fires the errored callback; otherwise after the window elapses the
// disconnected callback fires.
//
// Stretch extends the active window if the tmux reader reports lag.
type Debouncer struct {
	window       time.Duration
	onErrored    func(agentID string)
	onDisconnect func(agentID string)

	mu    sync.Mutex
	state map[string]*debounceEntry
}

type debounceEntry struct {
	timer    *time.Timer
	deadline time.Time
}

// NewDebouncer constructs a Debouncer with the given window and callbacks.
func NewDebouncer(window time.Duration, onErrored, onDisconnect func(string)) *Debouncer {
	if window <= 0 {
		window = DefaultDebounceWindow
	}
	return &Debouncer{
		window:       window,
		onErrored:    onErrored,
		onDisconnect: onDisconnect,
		state:        make(map[string]*debounceEntry),
	}
}

// OnEPIPE starts (or restarts) the debounce window for agentID.
func (d *Debouncer) OnEPIPE(agentID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if e, ok := d.state[agentID]; ok {
		e.timer.Stop()
	}
	deadline := time.Now().Add(d.window)
	t := time.AfterFunc(d.window, func() {
		d.mu.Lock()
		delete(d.state, agentID)
		d.mu.Unlock()
		if d.onDisconnect != nil {
			d.onDisconnect(agentID)
		}
	})
	d.state[agentID] = &debounceEntry{timer: t, deadline: deadline}
}

// OnPaneDied reports a pane_died event for agentID. If the EPIPE window is
// active, the errored callback fires; otherwise (no pending EPIPE) the
// errored callback still fires — pane death is itself terminal.
func (d *Debouncer) OnPaneDied(agentID string) {
	d.mu.Lock()
	if e, ok := d.state[agentID]; ok {
		e.timer.Stop()
		delete(d.state, agentID)
	}
	d.mu.Unlock()
	if d.onErrored != nil {
		d.onErrored(agentID)
	}
}

// Stretch lengthens the active window for agentID by d (no-op if no entry).
// Intended for the tmux reader goroutine to call when it detects lag.
func (db *Debouncer) Stretch(agentID string, d time.Duration) {
	db.mu.Lock()
	defer db.mu.Unlock()
	e, ok := db.state[agentID]
	if !ok {
		return
	}
	e.timer.Stop()
	e.deadline = e.deadline.Add(d)
	remaining := time.Until(e.deadline)
	if remaining <= 0 {
		remaining = time.Millisecond
	}
	e.timer = time.AfterFunc(remaining, func() {
		db.mu.Lock()
		delete(db.state, agentID)
		db.mu.Unlock()
		if db.onDisconnect != nil {
			db.onDisconnect(agentID)
		}
	})
}

// Stop cancels all pending timers.
func (d *Debouncer) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for id, e := range d.state {
		e.timer.Stop()
		delete(d.state, id)
	}
}
