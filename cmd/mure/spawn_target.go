package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// tmuxRunner is a seam over tmuxCmd that lets tests stub tmux invocations.
type tmuxRunner func(args ...string) (string, error)

// spawnTargetPlan describes the tmux invocation that should be used to create
// the new agent pane. Argv is passed to tmuxCmd by the caller. PostCreate, if
// non-nil, runs after the pane id is known and may issue follow-up tmux
// commands. It receives the returned pane_id.
type spawnTargetPlan struct {
	Argv       []string
	PostCreate func(paneID string) error
}

// pickSpawnTarget resolves @mure-spawn-target into a concrete plan.
// payload is the shell command string to exec in the new pane.
// stderr receives warnings (e.g. unknown target values).
func pickSpawnTarget(run tmuxRunner, target string, payload string, stderr io.Writer) (spawnTargetPlan, error) {
	switch target {
	case "new-window":
		return spawnTargetPlan{Argv: []string{"new-window", "-P", "-F", "#{pane_id}", payload}}, nil
	case "below-active":
		return spawnTargetPlan{Argv: []string{"split-window", "-v", "-P", "-F", "#{pane_id}", payload}}, nil
	case "right-of-active":
		return spawnTargetPlan{Argv: []string{"split-window", "-h", "-P", "-F", "#{pane_id}", payload}}, nil
	case "subagents-window", "":
		return planSubagentsWindow(run, payload)
	default:
		fmt.Fprintf(stderr, "mure spawn: unknown @mure-spawn-target %q; falling back to subagents-window\n", target)
		return planSubagentsWindow(run, payload)
	}
}

func planSubagentsWindow(run tmuxRunner, payload string) (spawnTargetPlan, error) {
	sessionID, err := resolveSessionID(run)
	if err != nil {
		return spawnTargetPlan{}, err
	}
	windowID, found, err := findSubagentsWindow(run, sessionID)
	if err != nil {
		return spawnTargetPlan{}, err
	}
	if found {
		return spawnTargetPlan{Argv: []string{"split-window", "-t", windowID, "-P", "-F", "#{pane_id}", payload}}, nil
	}
	return spawnTargetPlan{
		Argv: []string{"new-window", "-d", "-t", sessionID, "-n", "subagents", "-P", "-F", "#{pane_id}", payload},
		PostCreate: func(paneID string) error {
			wid, err := run("display-message", "-p", "-t", paneID, "#{window_id}")
			if err != nil {
				return err
			}
			_, err = run("set-option", "-w", "-t", wid, "@mure-subagents-window", "1")
			return err
		},
	}, nil
}

func resolveSessionID(run tmuxRunner) (string, error) {
	if pane := os.Getenv("TMUX_PANE"); pane != "" {
		return run("display-message", "-p", "-t", pane, "#{session_id}")
	}
	return run("display-message", "-p", "#{session_id}")
}

func findSubagentsWindow(run tmuxRunner, sessionID string) (string, bool, error) {
	out, err := run("list-windows", "-t", sessionID, "-F", "#{window_id} #{window_name} #{@mure-subagents-window}")
	if err != nil {
		return "", false, err
	}
	lines := strings.Split(out, "\n")
	type row struct {
		id, name, marker string
	}
	rows := make([]row, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		switch len(fields) {
		case 2:
			rows = append(rows, row{id: fields[0], name: fields[1]})
		case 3:
			rows = append(rows, row{id: fields[0], name: fields[1], marker: fields[2]})
		}
	}
	for _, r := range rows {
		if r.marker == "1" {
			return r.id, true, nil
		}
	}
	for _, r := range rows {
		if r.name == "subagents" {
			return r.id, true, nil
		}
	}
	return "", false, nil
}
