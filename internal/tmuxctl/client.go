// Package tmuxctl is a thin client for tmux control mode (`tmux -C`).
//
// It exposes a Client interface so the daemon can be unit-tested against a
// scripted fake (see fake.go). The real implementation spawns `tmux -C` and
// runs two goroutines (reader + writer) per PRD §6.1: one synchronous
// command in flight at a time, with all async `%`-notifications routed to
// Events().
package tmuxctl

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// Client speaks tmux control mode.
type Client interface {
	// Run sends a tmux command and returns its reply (the lines between
	// %begin and %end). If tmux replies with %error, Run returns a non-nil
	// error whose message includes the payload.
	Run(ctx context.Context, cmd string) (string, error)
	// Events returns the channel of async %-notifications. Closed on Close.
	Events() <-chan Event
	// Close terminates the underlying tmux process and goroutines.
	Close() error
}

// ErrClosed is returned by Run after Close.
var ErrClosed = errors.New("tmuxctl: client closed")

type replyResult struct {
	out string
	err error
}

type request struct {
	cmd string
	out chan replyResult
}

// pendingEntry is what the reader sees: where to deliver the reply, plus an
// ack the writer waits on so it doesn't pipeline the next command.
type pendingEntry struct {
	out chan replyResult
	ack chan struct{}
}

type realClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	events chan Event
	reqCh  chan request

	mu      sync.Mutex
	pending []pendingEntry // FIFO of in-flight reply entries
	closed  bool

	done chan struct{} // closed when reader exits
	wg   sync.WaitGroup
}

// Dial spawns `tmux -C <args...>` and returns a connected Client. Typical
// usage: Dial(ctx, "-L", "default", "attach", "-t", session) or
// Dial(ctx, "new-session", "-A", "-s", session).
func Dial(ctx context.Context, args ...string) (Client, error) {
	full := append([]string{"-C"}, args...)
	cmd := exec.CommandContext(ctx, "tmux", full...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return newClient(cmd, stdin, stdout), nil
}

func newClient(cmd *exec.Cmd, stdin io.WriteCloser, stdout io.ReadCloser) *realClient {
	c := &realClient{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		events: make(chan Event, 64),
		reqCh:  make(chan request),
		done:   make(chan struct{}),
	}
	c.wg.Add(2)
	go c.readerLoop()
	go c.writerLoop()
	return c
}

func (c *realClient) Events() <-chan Event { return c.events }

func (c *realClient) Run(ctx context.Context, cmd string) (string, error) {
	reply := make(chan replyResult, 1)
	select {
	case c.reqCh <- request{cmd: cmd, out: reply}:
	case <-c.done:
		return "", ErrClosed
	case <-ctx.Done():
		return "", ctx.Err()
	}
	select {
	case r := <-reply:
		return r.out, r.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (c *realClient) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	_ = c.stdin.Close()
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	_ = c.stdout.Close()
	c.wg.Wait()
	if c.cmd != nil {
		_ = c.cmd.Wait()
	}
	return nil
}

// writerLoop serializes commands. After writing a command it pushes the
// caller's reply channel onto the pending queue (FIFO) and blocks until the
// reader resolves it — guaranteeing one in-flight command at a time
// (PRD §6.1: no pipelining).
func (c *realClient) writerLoop() {
	defer c.wg.Done()
	for {
		select {
		case <-c.done:
			return
		case req := <-c.reqCh:
			c.mu.Lock()
			if c.closed {
				c.mu.Unlock()
				req.out <- replyResult{err: ErrClosed}
				continue
			}
			ack := make(chan struct{})
			c.pending = append(c.pending, pendingEntry{out: req.out, ack: ack})
			c.mu.Unlock()
			if _, err := io.WriteString(c.stdin, req.cmd+"\n"); err != nil {
				c.failPending(err)
				return
			}
			// Block until reader resolves this request: no pipelining.
			select {
			case <-c.done:
				return
			case <-ack:
			}
		}
	}
}

func (c *realClient) failPending(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range c.pending {
		p.out <- replyResult{err: err}
		close(p.ack)
	}
	c.pending = nil
}

// readerLoop scans stdout line by line. Lines between %begin N and the
// matching %end N / %error N are concatenated (with \n separators) into the
// reply payload. All other %-lines are routed to Events().
func (c *realClient) readerLoop() {
	defer c.wg.Done()
	defer close(c.done)
	defer close(c.events)

	sc := bufio.NewScanner(c.stdout)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)

	var inReply bool
	var payload strings.Builder

	for sc.Scan() {
		line := sc.Text()
		// Inside a %begin/%end block tmux only emits payload data, never
		// async %-notifications. Payload lines that happen to start with
		// '%' (e.g. pane ids like "%42" from list-panes -F '#{pane_id}')
		// must not be mistaken for control events.
		var ev Event
		var isCtl bool
		if !inReply {
			ev, isCtl = ParseLine(line)
		} else if strings.HasPrefix(line, "%end ") || strings.HasPrefix(line, "%error ") {
			ev, isCtl = ParseLine(line)
		}
		if !isCtl {
			if inReply {
				if payload.Len() > 0 {
					payload.WriteByte('\n')
				}
				payload.WriteString(line)
			}
			// Lines outside a reply block are dropped (shouldn't occur in
			// well-formed control-mode output).
			continue
		}
		switch ev.Kind {
		case EventBegin:
			inReply = true
			payload.Reset()
		case EventEnd:
			c.resolve(payload.String(), nil)
			payload.Reset()
			inReply = false
		case EventError:
			c.resolve("", fmt.Errorf("tmux: %s", payload.String()))
			payload.Reset()
			inReply = false
		default:
			// Async notification; non-blocking send so a stalled consumer
			// can't block the reader (drops oldest-style backpressure is a
			// daemon concern, not ours).
			select {
			case c.events <- ev:
			default:
			}
		}
	}
	// stdout closed; fail any pending replies.
	c.failPending(io.EOF)
}

func (c *realClient) resolve(out string, err error) {
	c.mu.Lock()
	if len(c.pending) == 0 {
		c.mu.Unlock()
		return
	}
	p := c.pending[0]
	c.pending = c.pending[1:]
	c.mu.Unlock()
	p.out <- replyResult{out: out, err: err}
	close(p.ack)
}
