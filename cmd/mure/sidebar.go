package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/audibleblink/mure/internal/sidebar"
)

func cmdSidebar(ctx context.Context, argv []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("sidebar", flag.ContinueOnError)
	fs.SetOutput(stderr)
	toggle := fs.Bool("toggle", false, "toggle the sidebar pane in the current tmux session")
	if err := fs.Parse(argv); err != nil {
		return 2
	}
	if *toggle {
		return runSidebarToggle(ctx, stderr)
	}
	// The sidebar pane is split with `-c '#{pane_current_path}'`, so our
	// cwd is already that of the invoking pane.
	if err := sidebar.Run(ctx); err != nil {
		fmt.Fprintln(stderr, "mure sidebar:", err)
		return 1
	}
	return 0
}

// runSidebarToggle finds a pane in the current session tagged
// `@mure-is-sidebar=1`; if present it's killed, otherwise a new pane is
// split running `mure sidebar` and tagged. Reads @mure-sidebar-width
// (default 36) and @mure-sidebar-position (default "left").
func runSidebarToggle(ctx context.Context, stderr *os.File) int {
	session, err := tmuxCmd(ctx, "display-message", "-p", "#{session_id}")
	if err != nil {
		fmt.Fprintln(stderr, "mure sidebar --toggle:", err)
		return 1
	}
	out, err := tmuxCmd(ctx, "list-panes", "-s", "-t", session,
		"-F", "#{pane_id} #{@mure-is-sidebar}")
	if err != nil {
		fmt.Fprintln(stderr, "mure sidebar --toggle:", err)
		return 1
	}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[1] == "1" {
			if _, err := tmuxCmd(ctx, "kill-pane", "-t", f[0]); err != nil {
				fmt.Fprintln(stderr, "mure sidebar --toggle:", err)
				return 1
			}
			return 0
		}
	}

	width := tmuxOption(ctx, "@mure-sidebar-width", "36")
	position := tmuxOption(ctx, "@mure-sidebar-position", "left")

	// -f makes tmux split the full window, so the sidebar spans the
	// entire edge regardless of the active pane's layout.
	var split []string
	switch position {
	case "right":
		split = []string{"-h", "-f"}
	case "top":
		split = []string{"-v", "-b", "-f"}
	case "bottom":
		split = []string{"-v", "-f"}
	default: // left
		split = []string{"-h", "-b", "-f"}
	}
	args := append([]string{"split-window", "-P", "-F", "#{pane_id}",
		"-c", "#{pane_current_path}"}, split...)
	args = append(args, "-l", width, "mure sidebar")

	paneID, err := tmuxCmd(ctx, args...)
	if err != nil {
		fmt.Fprintln(stderr, "mure sidebar --toggle:", err)
		return 1
	}
	if _, err := tmuxCmd(ctx, "set-option", "-p", "-t", paneID, "@mure-is-sidebar", "1"); err != nil {
		fmt.Fprintln(stderr, "mure sidebar --toggle:", err)
		return 1
	}
	return 0
}

func tmuxOption(ctx context.Context, name, def string) string {
	v, err := tmuxCmd(ctx, "show-option", "-gqv", name)
	if err != nil || v == "" {
		return def
	}
	return v
}
