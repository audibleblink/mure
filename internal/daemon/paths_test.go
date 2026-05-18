package daemon

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestRuntimeDir(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	}
	dir, err := RuntimeDir("test-session")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(dir, "/test-session") {
		t.Fatalf("unexpected dir: %s", dir)
	}
	st, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode %v, want 0700", st.Mode().Perm())
	}
}

func TestRuntimeDirEmptySession(t *testing.T) {
	if _, err := RuntimeDir(""); err == nil {
		t.Fatal("expected error for empty session")
	}
}
