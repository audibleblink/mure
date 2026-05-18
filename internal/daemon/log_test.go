package daemon

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLoggerRotates(t *testing.T) {
	dir := t.TempDir()
	const limit = 1024
	lg, err := NewLoggerWithLimit(dir, limit)
	if err != nil {
		t.Fatal(err)
	}
	defer lg.Close()

	chunk := bytes.Repeat([]byte("a"), 256)
	// Write 1.5KB worth: first 4 fit, the 5th triggers rotation.
	for i := 0; i < 6; i++ {
		if _, err := lg.Write(chunk); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	logPath := filepath.Join(dir, "daemon.log")
	rotPath := logPath + ".1"

	st, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("stat .log: %v", err)
	}
	if st.Size() >= limit {
		t.Fatalf(".log size %d not below limit %d after rotation", st.Size(), limit)
	}
	rs, err := os.Stat(rotPath)
	if err != nil {
		t.Fatalf("stat .log.1: %v", err)
	}
	if rs.Size() == 0 {
		t.Fatal(".log.1 should not be empty")
	}
}

func TestLoggerRotateOnce(t *testing.T) {
	// Verify multiple rotations only keep one .1 backup.
	dir := t.TempDir()
	lg, err := NewLoggerWithLimit(dir, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer lg.Close()
	for i := 0; i < 50; i++ {
		if _, err := lg.Write(bytes.Repeat([]byte("x"), 32)); err != nil {
			t.Fatal(err)
		}
	}
	entries, _ := os.ReadDir(dir)
	logs := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".log" || e.Name() == "daemon.log.1" {
			logs++
		}
	}
	if logs != 2 {
		t.Fatalf("expected 2 files (daemon.log + daemon.log.1), got %d: %v", logs, entries)
	}
}
