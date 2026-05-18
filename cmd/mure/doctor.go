package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
)

func cmdDoctor(ctx context.Context, _ []string, stdout, stderr *os.File) int {
	ok := true

	// tmux ≥ 3.2
	if path, err := exec.LookPath("tmux"); err != nil {
		fmt.Fprintln(stdout, "FAIL tmux: not found in PATH; install tmux 3.2+")
		ok = false
	} else if out, err := tmuxCmd(ctx, "-V"); err != nil {
		fmt.Fprintf(stdout, "FAIL tmux at %s: %v\n", path, err)
		ok = false
	} else if !tmuxAtLeast(out, 3, 2) {
		fmt.Fprintf(stdout, "FAIL tmux: %s; need 3.2+\n", out)
		ok = false
	} else {
		fmt.Fprintf(stdout, "OK   tmux: %s\n", out)
	}

	// plugin presence
	if v, err := tmuxCmd(ctx, "show-option", "-gv", "@mure-plugin-version"); err != nil || v == "" {
		fmt.Fprintln(stdout, "WARN tmux-mure plugin: not installed; add `set -g @plugin 'alex/mure'` and reload tmux")
	} else {
		fmt.Fprintf(stdout, "OK   tmux-mure plugin: v%s\n", v)
	}

	// socket path writable
	sockPath := resolveSocket()
	if sockPath == "" {
		fmt.Fprintln(stdout, "WARN MURE_SOCKET: not set (will be set by `mure up`)")
	} else {
		dir := filepath.Dir(sockPath)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			fmt.Fprintf(stdout, "FAIL socket dir %s: %v\n", dir, err)
			ok = false
		} else {
			fmt.Fprintf(stdout, "OK   socket dir writable: %s\n", dir)
		}
	}

	// peer-auth syscall availability (compile-time)
	switch runtime.GOOS {
	case "linux", "darwin":
		fmt.Fprintf(stdout, "OK   peer-auth: supported on %s\n", runtime.GOOS)
	default:
		fmt.Fprintf(stdout, "FAIL peer-auth: unsupported on %s\n", runtime.GOOS)
		ok = false
	}

	if !ok {
		fmt.Fprintln(stderr, "doctor: one or more checks failed")
		return 1
	}
	return 0
}

var tmuxVerRE = regexp.MustCompile(`(\d+)\.(\d+)`)

func tmuxAtLeast(v string, major, minor int) bool {
	m := tmuxVerRE.FindStringSubmatch(v)
	if len(m) != 3 {
		return false
	}
	maj, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	if maj != major {
		return maj > major
	}
	return min >= minor
}
