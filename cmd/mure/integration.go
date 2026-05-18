package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/audibleblink/mure/internal/piext"
)

func cmdIntegration(ctx context.Context, argv []string, stdout, stderr *os.File) int {
	if len(argv) < 2 {
		fmt.Fprintln(stderr, "usage: mure integration {install|uninstall} pi")
		return 2
	}
	action, target := argv[0], argv[1]
	if target != "pi" {
		fmt.Fprintf(stderr, "mure integration: unknown target %q\n", target)
		return 2
	}
	dest := piExtensionDir()
	switch action {
	case "install":
		if err := installPi(dest); err != nil {
			fmt.Fprintf(stderr, "mure integration install pi: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "installed: %s\n", dest)
	case "uninstall":
		if err := os.RemoveAll(dest); err != nil {
			fmt.Fprintf(stderr, "mure integration uninstall pi: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "removed: %s\n", dest)
	default:
		fmt.Fprintf(stderr, "mure integration: unknown action %q\n", action)
		return 2
	}
	_ = ctx
	return 0
}

func piExtensionDir() string {
	base := os.Getenv("PI_CODING_AGENT_DIR")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".pi", "agent")
	}
	return filepath.Join(base, "extensions", "mure")
}

func installPi(dest string) error {
	src := piext.FS()
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	return fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip hidden placeholders.
		if d.Name() == ".keep" {
			return nil
		}
		target := filepath.Join(dest, path)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := fs.ReadFile(src, path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}
