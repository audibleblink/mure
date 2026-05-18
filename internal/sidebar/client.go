package sidebar

import (
	"bufio"
	"context"
	"encoding/json"
	"math/rand"
	"net"
	"time"

	"github.com/audibleblink/mure/internal/sock"
)

// Frame is a typed message read from the daemon, surfaced to the model.
type Frame struct {
	Roster  *sock.Roster
	Update  *sock.AgentUpdate
	Connect bool // true = connected (after hello); false = disconnected
}

// Client maintains a connection to the daemon with exponential-backoff reconnect.
type Client struct {
	SocketPath string
	Frames     chan Frame

	// Dial is overridable in tests.
	Dial func(ctx context.Context, path string) (net.Conn, error)
	// Backoff overrides; if zero, defaults are used.
	BaseDelay time.Duration
	MaxDelay  time.Duration
}

// NewClient builds a client with default dialer and backoff.
func NewClient(socketPath string) *Client {
	return &Client{
		SocketPath: socketPath,
		Frames:     make(chan Frame, 64),
		Dial: func(ctx context.Context, p string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", p)
		},
		BaseDelay: 250 * time.Millisecond,
		MaxDelay:  30 * time.Second,
	}
}

// Run loops until ctx is cancelled, reconnecting with exponential backoff.
func (c *Client) Run(ctx context.Context) {
	defer close(c.Frames)
	delay := c.BaseDelay
	for {
		if ctx.Err() != nil {
			return
		}
		err := c.session(ctx)
		// On any session end (including a clean stream EOF), emit disconnect.
		c.send(ctx, Frame{Connect: false})
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			delay = c.BaseDelay
		} else {
			// Jittered backoff.
			j := time.Duration(rand.Int63n(int64(delay)))
			wait := delay/2 + j
			t := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-t.C:
			}
			delay *= 2
			if delay > c.MaxDelay {
				delay = c.MaxDelay
			}
		}
	}
}

func (c *Client) send(ctx context.Context, f Frame) {
	select {
	case <-ctx.Done():
	case c.Frames <- f:
	}
}

func (c *Client) session(ctx context.Context) error {
	conn, err := c.Dial(ctx, c.SocketPath)
	if err != nil {
		return err
	}
	defer conn.Close()
	// Close conn when ctx cancelled to unblock the reader.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()

	hello := sock.Hello{V: sock.ProtocolVersion, Event: "hello", Role: sock.RoleSidebar}
	if err := sock.WriteFrame(conn, hello); err != nil {
		return err
	}
	c.send(ctx, Frame{Connect: true})

	br := bufio.NewReader(conn)
	for {
		line, err := sock.ReadFrame(br, sock.MaxFrameSize)
		if err != nil {
			return err
		}
		event, _, err := sock.DecodeEnvelope(line)
		if err != nil {
			return err
		}
		switch event {
		case "roster":
			var r sock.Roster
			if err := json.Unmarshal(line, &r); err != nil {
				return err
			}
			c.send(ctx, Frame{Roster: &r})
		case "agent_update":
			var u sock.AgentUpdate
			if err := json.Unmarshal(line, &u); err != nil {
				return err
			}
			c.send(ctx, Frame{Update: &u})
		}
	}
}
