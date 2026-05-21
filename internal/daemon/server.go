package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/audibleblink/mure/internal/sock"
)

// stalePingTimeout bounds how long we'll wait for a hello-ack from an existing
// socket before deciding it's stale and unlinking it (PRD §6.4).
const stalePingTimeout = 200 * time.Millisecond

// Server is the Unix-socket front door. One per daemon process.
type Server struct {
	roster   *Roster
	listener net.Listener
	path     string

	mu       sync.Mutex
	conns    map[net.Conn]struct{}
	shutdown chan struct{}
}

// Shutdown asks Run to exit cleanly. Safe to call multiple times.
func (s *Server) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.shutdown:
	default:
		close(s.shutdown)
	}
	_ = s.listener.Close()
}

// Serve creates the listener at socketPath (with stale-socket cleanup), then
// blocks accepting connections until ctx is cancelled. The roster receives
// all agent/hook updates; subscribers (sidebar/cli) are fed from it.
func Serve(ctx context.Context, socketPath string, roster *Roster) error {
	s, err := Listen(socketPath, roster)
	if err != nil {
		return err
	}
	return s.Run(ctx)
}

// Listen binds the socket without starting the accept loop. Callers can use
// this to inspect Addr or to drive Run on their own goroutine.
func Listen(socketPath string, roster *Roster) (*Server, error) {
	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	// Tighten parent dir mode in case it pre-existed.
	_ = os.Chmod(dir, 0o700)

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		// Possibly a stale socket; try to ping it.
		if pingErr := pingSocket(socketPath, stalePingTimeout); pingErr != nil {
			_ = os.Remove(socketPath)
			ln, err = net.Listen("unix", socketPath)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("mure daemon already running at %s", socketPath)
		}
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		_ = ln.Close()
		return nil, err
	}
	return &Server{
		roster:   roster,
		listener: ln,
		path:     socketPath,
		conns:    make(map[net.Conn]struct{}),
		shutdown: make(chan struct{}),
	}, nil
}

// Addr returns the socket path the server is bound to.
func (s *Server) Addr() string { return s.path }

// Run accepts connections until ctx is cancelled or the listener closes.
func (s *Server) Run(ctx context.Context) error {
	go func() {
		select {
		case <-ctx.Done():
		case <-s.shutdown:
		}
		_ = s.listener.Close()
	}()
	defer func() {
		s.mu.Lock()
		for c := range s.conns {
			_ = c.Close()
		}
		s.mu.Unlock()
		_ = os.Remove(s.path)
	}()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
				return nil
			}
			return err
		}
		go s.handle(ctx, conn)
	}
}

func (s *Server) trackConn(c net.Conn, add bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if add {
		s.conns[c] = struct{}{}
	} else {
		delete(s.conns, c)
	}
}

func (s *Server) handle(ctx context.Context, conn net.Conn) {
	s.trackConn(conn, true)
	defer func() {
		_ = conn.Close()
		s.trackConn(conn, false)
	}()

	if err := Check(conn); err != nil {
		return
	}

	br := bufio.NewReader(conn)
	line, err := sock.ReadFrame(br, sock.MaxFrameSize)
	if err != nil {
		return
	}
	event, _, err := sock.DecodeEnvelope(line)
	if err != nil || event != "hello" {
		return
	}
	var h sock.Hello
	if err := json.Unmarshal(line, &h); err != nil {
		return
	}
	switch h.Role {
	case sock.RoleAgent:
		s.handleAgent(ctx, conn, br, h)
	case sock.RoleSidebar:
		s.handleSidebar(ctx, conn, br, h)
	case sock.RoleCLI:
		s.handleCLI(ctx, conn, br, h)
	default:
		// Unknown role: drop.
	}
}

func (s *Server) handleAgent(ctx context.Context, conn net.Conn, br *bufio.Reader, h sock.Hello) {
	if !h.Oneshot {
		s.roster.UpsertFromHello(h)
	}
	for {
		if ctx.Err() != nil {
			return
		}
		line, err := sock.ReadFrame(br, sock.MaxFrameSize)
		if err != nil {
			if !h.Oneshot {
				// Socket dropped without a graceful "bye" — the agent's pane
				// likely died (e.g. user pressed 'x' / kill-pane). Remove it.
				s.roster.Remove(h.AgentID)
			}
			return
		}
		event, _, err := sock.DecodeEnvelope(line)
		if err != nil {
			return
		}
		switch event {
		case "status":
			var st sock.Status
			if err := json.Unmarshal(line, &st); err != nil {
				return
			}
			s.roster.ApplyStatus(st)
		case "result":
			var rs sock.Result
			if err := json.Unmarshal(line, &rs); err != nil {
				return
			}
			s.roster.ApplyResult(rs)
		case "bye":
			s.roster.Remove(h.AgentID)
			return
		default:
			// Ignore unknown events on the agent stream.
		}
	}
}

func (s *Server) handleSidebar(ctx context.Context, conn net.Conn, br *bufio.Reader, _ sock.Hello) {
	// Send initial roster snapshot, then subscribe.
	if err := sock.WriteFrame(conn, s.roster.Snapshot()); err != nil {
		return
	}
	ch, cancel := s.roster.Subscribe()
	defer cancel()

	// Drain any client input (frames we don't currently use) on a side goroutine.
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			if _, err := sock.ReadFrame(br, sock.MaxFrameSize); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-readerDone:
			return
		case upd, ok := <-ch:
			if !ok {
				return
			}
			if err := sock.WriteFrame(conn, upd); err != nil {
				return
			}
		}
	}
}

func (s *Server) handleCLI(ctx context.Context, conn net.Conn, br *bufio.Reader, _ sock.Hello) {
	// Always answer the hello with a snapshot so ping-style probes succeed.
	if err := sock.WriteFrame(conn, s.roster.Snapshot()); err != nil {
		return
	}
	for {
		if ctx.Err() != nil {
			return
		}
		line, err := sock.ReadFrame(br, sock.MaxFrameSize)
		if err != nil {
			return
		}
		event, _, err := sock.DecodeEnvelope(line)
		if err != nil {
			return
		}
		switch event {
		case "shutdown":
			s.Shutdown()
			return
		case "snapshot":
			if err := sock.WriteFrame(conn, s.roster.Snapshot()); err != nil {
				return
			}
		case "register_pane":
			var rp sock.RegisterPane
			if err := json.Unmarshal(line, &rp); err != nil {
				return
			}
			s.roster.RegisterPane(rp)
		case "wait":
			var w sock.Wait
			if err := json.Unmarshal(line, &w); err != nil {
				return
			}
			s.handleWait(ctx, conn, w)
			return
		}
	}
}

// handleWait blocks until the agent identified by w.AgentID has a Result,
// then writes one AgentUpdate.
func (s *Server) handleWait(ctx context.Context, conn net.Conn, w sock.Wait) {
	terminal := func(a sock.AgentSnapshot) bool {
		return a.Result != ""
	}
	// Subscribe before snapshotting to avoid losing a transition that occurs
	// between the snapshot read and subscriber registration.
	ch, cancel := s.roster.Subscribe()
	defer cancel()
	snap := s.roster.Snapshot()
	for _, a := range snap.Agents {
		if a.ID == w.AgentID && terminal(a) {
			_ = sock.WriteFrame(conn, sock.AgentUpdate{V: sock.ProtocolVersion, Event: "agent_update", Agent: a})
			return
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case upd, ok := <-ch:
			if !ok {
				return
			}
			if upd.Agent.ID == w.AgentID && terminal(upd.Agent) {
				_ = sock.WriteFrame(conn, upd)
				return
			}
		}
	}
}

// pingSocket attempts a hello-handshake to determine if an existing socket has
// a live listener. Returns nil if the peer responded within timeout (i.e. it's
// alive), or an error otherwise (caller may then unlink the socket).
func pingSocket(path string, timeout time.Duration) error {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.Dial("unix", path)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	hello := sock.Hello{V: sock.ProtocolVersion, Event: "hello", Role: sock.RoleCLI}
	if err := sock.WriteFrame(conn, hello); err != nil {
		return err
	}
	br := bufio.NewReader(conn)
	if _, err := sock.ReadFrame(br, sock.MaxFrameSize); err != nil {
		return err
	}
	return nil
}
