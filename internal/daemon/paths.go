package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// RuntimeDir resolves and creates the per-session runtime directory.
// Linux:  $XDG_RUNTIME_DIR/mure/<session>/ (fallback /tmp/mure-<uid>/<session>/)
// macOS:  ~/Library/Caches/mure/<session>/
// (PRD §15)
func RuntimeDir(session string) (string, error) {
	if session == "" {
		return "", fmt.Errorf("mure: empty session name")
	}
	var base string
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, "Library", "Caches", "mure")
	default:
		if x := os.Getenv("XDG_RUNTIME_DIR"); x != "" {
			base = filepath.Join(x, "mure")
		} else {
			base = filepath.Join(os.TempDir(), fmt.Sprintf("mure-%d", os.Getuid()))
		}
	}
	p := filepath.Join(base, session)
	if err := os.MkdirAll(p, 0o700); err != nil {
		return "", err
	}
	_ = os.Chmod(p, 0o700)
	return p, nil
}

// SocketPath returns the canonical Unix socket path under runDir.
func SocketPath(runDir string) string { return filepath.Join(runDir, "daemon.sock") }
