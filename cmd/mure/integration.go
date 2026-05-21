package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/audibleblink/mure/internal/harnesses"
)

func cmdIntegration(ctx context.Context, argv []string, stdout, stderr *os.File) int {
	_ = ctx
	if len(argv) < 1 {
		fmt.Fprintln(stderr, "usage: mure integration {list|install <name>|uninstall <name>}")
		return 2
	}
	manifests, loadErr := harnesses.Load(harnesses.SourceFS())
	// loadErr is non-fatal for listing; surface for install/uninstall.

	switch argv[0] {
	case "list":
		if loadErr != nil {
			fmt.Fprintf(stderr, "warning: %v\n", loadErr)
		}
		return integrationList(manifests, stdout)
	case "install":
		if loadErr != nil {
			fmt.Fprintf(stderr, "mure integration: %v\n", loadErr)
			return 1
		}
		if len(argv) < 2 {
			fmt.Fprintln(stderr, "usage: mure integration install <name>")
			return 2
		}
		return integrationInstall(manifests, argv[1], stdout, stderr)
	case "uninstall":
		if len(argv) < 2 {
			fmt.Fprintln(stderr, "usage: mure integration uninstall <name>")
			return 2
		}
		return integrationUninstall(argv[1], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "mure integration: unknown action %q\n", argv[0])
		return 2
	}
}

func integrationList(ms []harnesses.Manifest, stdout *os.File) int {
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tDISPLAY\tINSTALLED\tSPAWN\tSTATUS\tRESULT\tNOTE")
	for _, m := range ms {
		_, installed, _ := harnesses.ReadState(m.Name)
		note := ""
		if !m.Capabilities.Status || !m.Capabilities.Result {
			note = "degraded"
		}
		fmt.Fprintf(tw, "%s\t%s\t%v\t%v\t%v\t%v\t%s\n",
			m.Name, m.Display, installed,
			m.Capabilities.Spawn, m.Capabilities.Status, m.Capabilities.Result, note)
	}
	tw.Flush()
	return 0
}

func integrationInstall(ms []harnesses.Manifest, name string, stdout, stderr *os.File) int {
	m, ok := harnesses.Get(ms, name)
	if !ok {
		fmt.Fprintf(stderr, "mure integration: unknown harness %q\n", name)
		return 2
	}
	ops, err := harnesses.BuildPlan(m, harnesses.SourceFS())
	if err != nil {
		fmt.Fprintf(stderr, "mure integration install %s: %v\n", name, err)
		return 1
	}
	r, err := harnesses.Apply(name, ops)
	if err != nil {
		fmt.Fprintf(stderr, "mure integration install %s: %v\n", name, err)
		return 1
	}
	if err := harnesses.WriteState(name, r); err != nil {
		fmt.Fprintf(stderr, "mure integration install %s: %v\n", name, err)
		return 1
	}
	fmt.Fprintf(stdout, "installed: %s (%d files)\n", name, len(r.Files))
	return 0
}

func integrationUninstall(name string, stdout, stderr *os.File) int {
	r, ok, err := harnesses.ReadState(name)
	if err != nil {
		fmt.Fprintf(stderr, "mure integration uninstall %s: %v\n", name, err)
		return 1
	}
	if !ok {
		fmt.Fprintf(stdout, "not installed: %s\n", name)
		return 0
	}
	warns := harnesses.Uninstall(r)
	for _, w := range warns {
		fmt.Fprintf(stderr, "warning: %v\n", w)
	}
	if err := harnesses.ClearState(name); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(stderr, "mure integration uninstall %s: %v\n", name, err)
		return 1
	}
	fmt.Fprintf(stdout, "removed: %s\n", name)
	return 0
}
