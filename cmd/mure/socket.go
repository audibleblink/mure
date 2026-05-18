package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/audibleblink/mure/internal/daemon"
	"github.com/audibleblink/mure/internal/sock"
)

// resolveSocket returns $MURE_SOCKET, or the canonical per-session daemon
// socket path when MURE_SOCKET is unset.
func resolveSocket() string {
	if p := os.Getenv("MURE_SOCKET"); p != "" {
		return p
	}
	session := resolveSession()
	runDir, err := daemon.RuntimeDir(session)
	if err != nil {
		return ""
	}
	return daemon.SocketPath(runDir)
}

func resolveSession() string {
	if s := os.Getenv("MURE_SESSION"); s != "" {
		return s
	}
	if os.Getenv("TMUX") != "" {
		args := []string{"display-message", "-p", "#S"}
		if s := os.Getenv("MURE_TMUX_SOCKET"); s != "" {
			args = append([]string{"-S", s}, args...)
		}
		cmd := exec.Command("tmux", args...)
		if out, err := cmd.Output(); err == nil && strings.TrimSpace(string(out)) != "" {
			return strings.TrimSpace(string(out))
		}
	}
	return "default"
}

// dialHello connects as the given role and sends a hello frame.
func dialHello(path, role string, timeout time.Duration) (net.Conn, *bufio.Reader, error) {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.Dial("unix", path)
	if err != nil {
		return nil, nil, err
	}
	if err := sock.WriteFrame(conn, sock.Hello{V: sock.ProtocolVersion, Event: "hello", Role: role}); err != nil {
		conn.Close()
		return nil, nil, err
	}
	return conn, bufio.NewReader(conn), nil
}

// pingDaemon connects to path and reads one frame back to confirm health.
func pingDaemon(ctx context.Context, path string) error {
	if path == "" {
		return errors.New("MURE_SOCKET not set")
	}
	dl, ok := ctx.Deadline()
	if !ok {
		dl = time.Now().Add(500 * time.Millisecond)
	}
	conn, br, err := dialHello(path, sock.RoleCLI, time.Until(dl))
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(dl)
	if _, err := sock.ReadFrame(br, sock.MaxFrameSize); err != nil {
		return err
	}
	return nil
}

// tmuxCmd runs `tmux <args...>` and returns stdout (trimmed) or an error
// whose message includes stderr. Honors $MURE_TMUX_SOCKET so tests (and
// daemons attached to a non-default tmux server) route commands correctly.
func tmuxCmd(ctx context.Context, args ...string) (string, error) {
	if s := os.Getenv("MURE_TMUX_SOCKET"); s != "" {
		args = append([]string{"-S", s}, args...)
	}
	cmd := exec.CommandContext(ctx, "tmux", args...)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("tmux %s: %s", strings.Join(args, " "), strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
