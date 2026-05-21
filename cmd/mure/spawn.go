package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/audibleblink/mure/internal/harnesses"
)

func cmdSpawn(ctx context.Context, argv []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("spawn", flag.ContinueOnError)
	fs.SetOutput(stderr)
	harnessFlag := fs.String("harness", "", "harness name (overrides MURE_HARNESS / @mure-harness)")
	if err := fs.Parse(argv); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) < 1 {
		fmt.Fprintln(stderr, "usage: mure spawn [--harness <name>] <role> [task]")
		return 2
	}
	role := rest[0]
	task := strings.Join(rest[1:], " ")

	sockPath := resolveSocket()
	if sockPath == "" {
		fmt.Fprintln(stderr, "mure spawn: MURE_SOCKET not set")
		return 1
	}

	run := tmuxRunnerFromCtx(ctx)
	harnessName, err := resolveHarness(*harnessFlag, os.Getenv("MURE_HARNESS"), run)
	if err != nil {
		fmt.Fprintf(stderr, "mure spawn: %v\n", err)
		return 1
	}

	manifests, err := harnesses.Load(harnesses.SourceFS())
	if err != nil {
		fmt.Fprintf(stderr, "mure spawn: load harnesses: %v\n", err)
		return 1
	}
	m, ok := harnesses.Get(manifests, harnessName)
	if !ok {
		fmt.Fprintf(stderr, "mure spawn: unknown harness %q\n", harnessName)
		return 1
	}
	ta, err := harnesses.ParseTaskArg(m.TaskArg)
	if err != nil {
		fmt.Fprintf(stderr, "mure spawn: %v\n", err)
		return 1
	}

	target, err := tmuxCmd(ctx, "show-option", "-gv", "@mure-spawn-target")
	if err != nil {
		target = ""
	}

	agentID := newAgentID()
	session := resolveSession()
	payload := spawnPayload(spawnEnv{
		AgentID:     agentID,
		SockPath:    sockPath,
		Role:        role,
		Task:        task,
		Session:     session,
		HarnessName: m.Name,
		Command:     m.Command,
		TaskArg:     ta,
	}, os.Environ())

	plan, err := pickSpawnTarget(run, target, payload)
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

// resolveHarness checks the four configuration slots in order:
//
//	--harness flag → MURE_HARNESS env → tmux session @mure-harness → tmux global @mure-harness.
func resolveHarness(flagVal, envVal string, run tmuxRunner) (string, error) {
	if flagVal != "" {
		return flagVal, nil
	}
	if envVal != "" {
		return envVal, nil
	}
	if v, err := run("show-option", "-qv", "@mure-harness"); err == nil && v != "" {
		return v, nil
	}
	if v, err := run("show-option", "-gqv", "@mure-harness"); err == nil && v != "" {
		return v, nil
	}
	return "", fmt.Errorf("no harness configured (tried: --harness flag, MURE_HARNESS env, tmux session @mure-harness, tmux global @mure-harness)")
}

func tmuxRunnerFromCtx(ctx context.Context) tmuxRunner {
	return func(args ...string) (string, error) { return tmuxCmd(ctx, args...) }
}

func newAgentID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return "agent-" + hex.EncodeToString(b[:])
}

type spawnEnv struct {
	AgentID     string
	SockPath    string
	Role        string
	Task        string
	Session     string
	HarnessName string
	Command     string
	TaskArg     harnesses.TaskArg
}

func spawnPayload(se spawnEnv, environ []string) string {
	assignments := make([]string, 0, len(environ)+8)
	for _, kv := range environ {
		name, value, ok := strings.Cut(kv, "=")
		if !ok || name == "" {
			continue
		}
		if name == "TMUX_PANE" {
			continue
		}
		assignments = append(assignments, name+"="+shellEscape(value))
	}
	assignments = append(assignments,
		"MURE_ENV=1",
		"MURE_AGENT_ID="+shellEscape(se.AgentID),
		"MURE_SOCKET="+shellEscape(se.SockPath),
		"MURE_HARNESS="+shellEscape(se.HarnessName),
		// MURE_PANE_ID resolves at shell startup from tmux-provided TMUX_PANE.
		`MURE_PANE_ID="$TMUX_PANE"`,
	)
	if se.Session != "" {
		assignments = append(assignments, "MURE_SESSION="+shellEscape(se.Session))
	}
	if se.Role != "" {
		assignments = append(assignments, "MURE_ROLE="+shellEscape(se.Role))
	}
	cmd := se.Command
	if se.Task != "" {
		assignments = append(assignments, "MURE_TASK="+shellEscape(se.Task))
		switch se.TaskArg.Kind {
		case harnesses.TaskArgPositional:
			cmd = se.Command + " " + shellEscape(se.Task)
		case harnesses.TaskArgFlag:
			cmd = se.Command + " " + shellEscape("--"+strings.TrimPrefix(se.TaskArg.Flag, "--")) + " " + shellEscape(se.Task)
		case harnesses.TaskArgStdin:
			cmd = fmt.Sprintf("printf %%s %s | %s", shellEscape(se.Task), se.Command)
		}
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
