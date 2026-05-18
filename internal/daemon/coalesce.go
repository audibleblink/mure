package daemon

import (
	"sync"
	"time"
)

// DefaultCoalesceWindow is the per-(pane, option) write debounce (PRD §6.1).
const DefaultCoalesceWindow = 500 * time.Millisecond

// CoalesceWrite is one queued pane-option write.
type CoalesceWrite struct {
	PaneID string
	Option string
	Value  string
}

// Coalescer batches repeated writes to the same (paneID, option) within a
// window into a single emission carrying the latest value.
type Coalescer struct {
	window time.Duration
	out    chan CoalesceWrite

	mu          sync.Mutex
	pending     map[string]*coalEntry
	lastEmitted map[string]string
	closed      bool
}

type coalEntry struct {
	timer *time.Timer
	value string
}

// NewCoalescer constructs a Coalescer with the given window. Reads happen on
// the channel returned by Out(); the buffer is sized for typical bursts.
func NewCoalescer(window time.Duration) *Coalescer {
	if window <= 0 {
		window = DefaultCoalesceWindow
	}
	return &Coalescer{
		window:      window,
		out:         make(chan CoalesceWrite, 64),
		pending:     make(map[string]*coalEntry),
		lastEmitted: make(map[string]string),
	}
}

// Out returns the channel onto which coalesced writes are emitted.
func (c *Coalescer) Out() <-chan CoalesceWrite { return c.out }

// Submit queues a write; the value supersedes any pending value for the same
// (paneID, option) key.
func (c *Coalescer) Submit(paneID, option, value string) {
	key := paneID + "\x00" + option
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	if e, ok := c.pending[key]; ok {
		e.value = value
		return
	}
	e := &coalEntry{value: value}
	e.timer = time.AfterFunc(c.window, func() { c.flush(key, paneID, option) })
	c.pending[key] = e
}

func (c *Coalescer) flush(key, paneID, option string) {
	c.mu.Lock()
	e, ok := c.pending[key]
	if !ok {
		c.mu.Unlock()
		return
	}
	delete(c.pending, key)
	value := e.value
	closed := c.closed
	last, hadLast := c.lastEmitted[key]
	if !closed {
		c.lastEmitted[key] = value
	}
	c.mu.Unlock()
	if closed || (hadLast && last == value) {
		return
	}
	c.out <- CoalesceWrite{PaneID: paneID, Option: option, Value: value}
}

// Close stops all pending timers and closes the output channel.
func (c *Coalescer) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	for k, e := range c.pending {
		e.timer.Stop()
		delete(c.pending, k)
	}
	c.mu.Unlock()
	close(c.out)
}
