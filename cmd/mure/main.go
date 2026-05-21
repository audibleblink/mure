package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// verbFunc is one CLI verb. argv excludes the verb itself.
type verbFunc func(ctx context.Context, argv []string, stdout, stderr *os.File) int

var verbs = map[string]verbFunc{
	"up":          cmdUp,
	"down":        cmdDown,
	"ls":          cmdLs,
	"spawn":       cmdSpawn,
	"wait":        cmdWait,
	"focus":       cmdFocus,
	"sidebar":     cmdSidebar,
	"doctor":      cmdDoctor,
	"integration": cmdIntegration,
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		printUsage(stdout)
		return 0
	}
	verb := args[0]
	if verb == "-h" || verb == "--help" || verb == "help" {
		printUsage(stdout)
		return 0
	}
	fn, ok := verbs[verb]
	if !ok {
		fmt.Fprintf(stderr, "mure: unknown verb %q\n", verb)
		printUsage(stderr)
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return fn(ctx, args[1:], stdout, stderr)
}

func printUsage(w *os.File) {
	fmt.Fprintln(w, `usage: mure <verb> [args...]

verbs:
  up                          start daemon
  down                        stop daemon
  ls [--json]                 list agents
  spawn <role> [task]         spawn an agent pane
  wait <agent>                wait for agent's final result
  focus <agent>               select pane for agent
  sidebar                     run sidebar TUI
  doctor                      diagnostics
  integration install pi      install pi extension
  integration uninstall pi    uninstall pi extension`)
}
