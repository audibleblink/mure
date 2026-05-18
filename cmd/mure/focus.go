package main

import (
	"context"
	"fmt"
	"os"
	"strings"
)

func cmdFocus(ctx context.Context, argv []string, stdout, stderr *os.File) int {
	if len(argv) != 1 {
		fmt.Fprintln(stderr, "usage: mure focus <agent>")
		return 2
	}
	agent := argv[0]
	pane := agent
	if !strings.HasPrefix(agent, "%") {
		// Resolve agent id → pane via tmux: scan panes with @mure-agent-id.
		out, err := tmuxCmd(ctx, "list-panes", "-a", "-F", "#{pane_id} #{@mure-agent-id}")
		if err != nil {
			fmt.Fprintf(stderr, "mure focus: %v\n", err)
			return 1
		}
		pane = ""
		for _, line := range strings.Split(out, "\n") {
			parts := strings.SplitN(line, " ", 2)
			if len(parts) == 2 && parts[1] == agent {
				pane = parts[0]
				break
			}
		}
		if pane == "" {
			fmt.Fprintf(stderr, "mure focus: agent %q not found\n", agent)
			return 1
		}
	}
	if _, err := tmuxCmd(ctx, "select-pane", "-t", pane); err != nil {
		fmt.Fprintf(stderr, "mure focus: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, pane)
	return 0
}
