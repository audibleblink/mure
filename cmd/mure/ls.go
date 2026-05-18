package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/audibleblink/mure/internal/sock"
)

func cmdLs(ctx context.Context, argv []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("ls", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "emit raw roster JSON")
	if err := fs.Parse(argv); err != nil {
		return 2
	}
	path := resolveSocket()
	if path == "" {
		fmt.Fprintln(stderr, "mure ls: MURE_SOCKET not set")
		return 1
	}
	conn, br, err := dialHello(path, sock.RoleCLI, time.Second)
	if err != nil {
		fmt.Fprintf(stderr, "mure ls: %v\n", err)
		return 1
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	line, err := sock.ReadFrame(br, sock.MaxFrameSize)
	if err != nil {
		fmt.Fprintf(stderr, "mure ls: %v\n", err)
		return 1
	}
	if *jsonOut {
		stdout.Write(line)
		if len(line) == 0 || line[len(line)-1] != '\n' {
			stdout.Write([]byte("\n"))
		}
		return 0
	}
	var r sock.Roster
	if err := json.Unmarshal(line, &r); err != nil {
		fmt.Fprintf(stderr, "mure ls: decode: %v\n", err)
		return 1
	}
	_ = ctx
	return renderRoster(stdout, r)
}

func renderRoster(w io.Writer, r sock.Roster) int {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AGENT\tSTATUS\tPANE\tTASK")
	agents := append([]sock.AgentSnapshot(nil), r.Agents...)
	sort.Slice(agents, func(i, j int) bool { return agents[i].ID < agents[j].ID })
	for _, a := range agents {
		name := a.ID
		if a.Role != "" {
			name = a.Role
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", name, a.Status, a.Pane, a.Task)
	}
	return errExit(tw.Flush())
}

func errExit(err error) int {
	if err != nil {
		return 1
	}
	return 0
}
