package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/audibleblink/mure/internal/daemon"
	"github.com/audibleblink/mure/internal/tmuxctl"
)

// cmdUp handles `mure up`. Re-entrant: ping first, exit 0 with
// "already running" if a healthy daemon is at $MURE_SOCKET.
//
// When MURE_DAEMON=1 the process becomes the daemon (used by the fork
// path); otherwise it forks itself in the background.
func cmdUp(ctx context.Context, _ []string, stdout, stderr *os.File) int {
	if os.Getenv("MURE_DAEMON") == "1" {
		return runDaemon(ctx, stderr)
	}

	sockPath := resolveSocket()
	if sockPath != "" {
		if err := pingDaemon(ctx, sockPath); err == nil {
			fmt.Fprintln(stdout, "already running")
			return 0
		}
	}

	// Resolve tmux server socket + session name from TMUX env so the daemon
	// can attach as a tmuxctl client.
	envExtra := []string{"MURE_DAEMON=1"}
	if os.Getenv("MURE_LAUNCH_DIR") == "" {
		if cwd, err := os.Getwd(); err == nil && cwd != "" {
			envExtra = append(envExtra, "MURE_LAUNCH_DIR="+cwd)
		}
	}
	if tmuxEnv := os.Getenv("TMUX"); tmuxEnv != "" {
		parts := strings.SplitN(tmuxEnv, ",", 2)
		if parts[0] != "" && os.Getenv("MURE_TMUX_SOCKET") == "" {
			envExtra = append(envExtra, "MURE_TMUX_SOCKET="+parts[0])
		}
		if os.Getenv("MURE_SESSION") == "" {
			if name, err := tmuxCmd(ctx, "display-message", "-p", "#S"); err == nil && name != "" {
				envExtra = append(envExtra, "MURE_SESSION="+name)
			}
		}
	}

	// Fork self with MURE_DAEMON=1.
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "mure up: cannot locate executable: %v\n", err)
		return 1
	}
	cmd := exec.Command(exe, "up")
	cmd.Env = append(os.Environ(), envExtra...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(stderr, "mure up: fork: %v\n", err)
		return 1
	}
	_ = cmd.Process.Release()
	fmt.Fprintln(stdout, "started")
	return 0
}

// runDaemon is the MURE_DAEMON=1 branch: start a daemon without tmux
// integration unless a session is identified via $MURE_SESSION.
func runDaemon(ctx context.Context, stderr *os.File) int {
	session := os.Getenv("MURE_SESSION")
	if session == "" {
		session = "default"
	}
	runDir, err := daemon.RuntimeDir(session)
	if err != nil {
		fmt.Fprintf(stderr, "mure up: rundir: %v\n", err)
		return 1
	}
	sockPath := os.Getenv("MURE_SOCKET")
	if sockPath == "" {
		sockPath = daemon.SocketPath(runDir)
	}
	cfg := daemon.Config{Session: session, SocketPath: sockPath, RunDir: runDir, LaunchDir: os.Getenv("MURE_LAUNCH_DIR")}
	if tmuxSock := os.Getenv("MURE_TMUX_SOCKET"); tmuxSock != "" {
		c, err := tmuxctl.Dial(ctx, "-S", tmuxSock, "attach", "-t", session)
		if err != nil {
			fmt.Fprintf(stderr, "mure up: tmux: %v\n", err)
			return 1
		}
		cfg.Tmux = c
	}
	if err := daemon.Run(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(stderr, "mure up: %v\n", err)
		return 1
	}
	return 0
}
