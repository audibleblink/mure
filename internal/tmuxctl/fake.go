package tmuxctl

import (
	"context"
	"errors"
	"sync"
)

// ScriptedFake is a test Client. Tests pre-queue replies (one per expected
// Run call) and push async events via EmitEvent.
type ScriptedFake struct {
	mu       sync.Mutex
	replies  []replyResult
	commands []string // commands seen, in Run order
	events   chan Event
	closed   bool
}

// NewScriptedFake returns a Client suitable for tests.
func NewScriptedFake() *ScriptedFake {
	return &ScriptedFake{events: make(chan Event, 64)}
}

// EnqueueReply queues the next reply that Run will return (FIFO).
func (f *ScriptedFake) EnqueueReply(out string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replies = append(f.replies, replyResult{out: out, err: err})
}

// EmitEvent pushes an async event to Events().
func (f *ScriptedFake) EmitEvent(e Event) {
	f.events <- e
}

// Commands returns the commands seen by Run in order.
func (f *ScriptedFake) Commands() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.commands...)
}

func (f *ScriptedFake) Run(_ context.Context, cmd string) (string, error) {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return "", ErrClosed
	}
	if len(f.replies) == 0 {
		f.mu.Unlock()
		return "", errors.New("tmuxctl: no scripted reply for " + cmd)
	}
	r := f.replies[0]
	f.replies = f.replies[1:]
	f.commands = append(f.commands, cmd)
	f.mu.Unlock()
	return r.out, r.err
}

func (f *ScriptedFake) Events() <-chan Event { return f.events }

func (f *ScriptedFake) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil
	}
	f.closed = true
	close(f.events)
	return nil
}
