package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"
)

func cmdSpawn(ctx context.Context, argv []string, stdout, stderr *os.File) int {
	if len(argv) < 1 {
		fmt.Fprintln(stderr, "usage: mure spawn <role> [task]")
		return 2
	}
	role := argv[0]
	task := strings.Join(argv[1:], " ")

	sockPath := resolveSocket()
	if sockPath == "" {
		fmt.Fprintln(stderr, "mure spawn: MURE_SOCKET not set")
		return 1
	}

	target, err := tmuxCmd(ctx, "show-option", "-gv", "@mure-spawn-target")
	if err != nil {
		target = ""
	}

	agentCmd := os.Getenv("MURE_AGENT_CMD")
	if agentCmd == "" {
		agentCmd = "pi"
	}
	agentID := newAgentID()

	// Build shell payload: pass through the current environment, then add
	// mure-specific values for the spawned pane.
	payload := spawnPayload(agentID, sockPath, role, task, agentCmd, os.Environ())

	plan, err := pickSpawnTarget(tmuxRunnerFromCtx(ctx), target, payload, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "mure spawn: %v\n", err)
		return 1
	}
	paneID, err := tmuxCmd(ctx, plan.Argv...)
	if err != nil {
		fmt.Fprintf(stderr, "mure spawn: %v\n", err)
		return 1
	}
	if plan.PostCreate != nil {
		if err := plan.PostCreate(paneID); err != nil {
			fmt.Fprintf(stderr, "mure spawn: %v\n", err)
			return 1
		}
	}

	now := fmt.Sprintf("%d", time.Now().UnixMilli())
	if _, err := tmuxCmd(ctx, "set-option", "-p", "-t", paneID, "@mure-role", role); err != nil {
		fmt.Fprintf(stderr, "mure spawn: %v\n", err)
		return 1
	}
	if _, err := tmuxCmd(ctx, "set-option", "-p", "-t", paneID, "@mure-spawned-at", now); err != nil {
		fmt.Fprintf(stderr, "mure spawn: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "%s %s\n", agentID, paneID)
	return 0
}

func tmuxRunnerFromCtx(ctx context.Context) tmuxRunner {
	return func(args ...string) (string, error) { return tmuxCmd(ctx, args...) }
}

func newAgentID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return "agent-" + hex.EncodeToString(b[:])
}

func spawnPayload(agentID, sockPath, role, task, agentCmd string, environ []string) string {
	assignments := make([]string, 0, len(environ)+5)
	for _, kv := range environ {
		name, value, ok := strings.Cut(kv, "=")
		if !ok || name == "" {
			continue
		}
		// Drop TMUX_PANE so the new pane gets its own value from tmux.
		if name == "TMUX_PANE" {
			continue
		}
		assignments = append(assignments, name+"="+shellEscape(value))
	}
	assignments = append(assignments,
		"MURE_ENV=1",
		"MURE_AGENT_ID="+shellEscape(agentID),
		"MURE_SOCKET="+shellEscape(sockPath),
	)
	if role != "" {
		assignments = append(assignments, "MURE_ROLE="+shellEscape(role))
	}
	cmd := agentCmd
	if task != "" {
		assignments = append(assignments, "MURE_TASK="+shellEscape(task))
		// Pass task as a positional argument so pi processes it as the initial prompt.
		cmd = agentCmd + " " + shellEscape(task)
	}
	inner := fmt.Sprintf("exec env %s %s", strings.Join(assignments, " "), cmd)
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	return fmt.Sprintf("%s -lic %s", shell, shellEscape(inner))
}

// shellEscape wraps a value in single quotes for safe inclusion in a shell
// command string handed to tmux.
func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
