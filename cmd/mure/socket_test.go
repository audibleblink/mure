package main

import (
	"path/filepath"
	"testing"

	"github.com/audibleblink/mure/internal/daemon"
)

func TestResolveSocketDefaultsToSessionRuntimeDir(t *testing.T) {
	t.Setenv("MURE_SOCKET", "")
	t.Setenv("MURE_SESSION", "unit-session")
	t.Setenv("TMUX", "")

	got := resolveSocket()
	runDir, err := daemon.RuntimeDir("unit-session")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(runDir, "daemon.sock")
	if got != want {
		t.Fatalf("resolveSocket()=%q, want %q", got, want)
	}
}

func TestResolveSocketHonorsEnvOverride(t *testing.T) {
	t.Setenv("MURE_SOCKET", "/tmp/custom.sock")
	t.Setenv("MURE_SESSION", "unit-session")

	if got := resolveSocket(); got != "/tmp/custom.sock" {
		t.Fatalf("resolveSocket()=%q", got)
	}
}
