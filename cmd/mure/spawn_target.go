package main

import (
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
//
// The option is either the reserved keyword "subagents-window" (or empty,
// same meaning) — which triggers find-or-create of a dedicated window — or
// an arbitrary tmux command template (e.g. "split-window -h",
// "new-window", "split-window -h -f -l 40%"). For templates, mure appends
// `-P -F #{pane_id} <payload>` and runs them.
//
// Legacy keyword values (right-of-active, below-active, new-window) are
// rewritten to their command equivalents by the tmux plugin at load time,
// so they do not appear here.
func pickSpawnTarget(run tmuxRunner, target string, payload string) (spawnTargetPlan, error) {
	if target == "" || target == "subagents-window" {
		return planSubagentsWindow(run, payload)
	}
	fields := strings.Fields(target)
	if len(fields) == 0 {
		return planSubagentsWindow(run, payload)
	}
	argv := append(fields, "-P", "-F", "#{pane_id}", payload)
	return spawnTargetPlan{Argv: argv}, nil
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
		return spawnTargetPlan{
			Argv: []string{"split-window", "-h", "-t", windowID, "-P", "-F", "#{pane_id}", payload},
			PostCreate: func(paneID string) error {
				_, err := run("select-layout", "-t", windowID, "even-horizontal")
				return err
			},
		}, nil
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
