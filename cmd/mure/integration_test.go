package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/audibleblink/mure/internal/harnesses"
)

func setupTestHarness(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	harnesses.SetFSForTesting(fstest.MapFS{
		"demo/manifest.toml": {Data: []byte("name = \"demo\"\ndisplay = \"Demo\"\ncommand = \"demo\"\n" +
			"[capabilities]\nspawn = true\nstatus = true\nresult = true\n" +
			"[install.skill]\npath = \"~/skill.md\"\nmerge = \"append\"\n" +
			"[[install.files]]\nsrc = \"h.sh\"\ndst = \"~/h.sh\"\nmode = \"0755\"\n")},
		"demo/skill.md": {Data: []byte("body")},
		"demo/h.sh":     {Data: []byte("#!/bin/sh\n")},
	})
	t.Cleanup(func() { harnesses.SetFSForTesting(nil) })
}

func runIntegration(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	outR, outW, _ := os.Pipe()
	errR, errW, _ := os.Pipe()
	code := cmdIntegration(context.Background(), args, outW, errW)
	outW.Close()
	errW.Close()
	var o, e bytes.Buffer
	o.ReadFrom(outR)
	e.ReadFrom(errR)
	return code, o.String(), e.String()
}

func TestIntegration_InstallListUninstall(t *testing.T) {
	setupTestHarness(t)

	if c, _, e := runIntegration(t, "list"); c != 0 {
		t.Fatalf("list pre: %d %s", c, e)
	}

	code, out, errOut := runIntegration(t, "install", "demo")
	if code != 0 {
		t.Fatalf("install: code=%d err=%s", code, errOut)
	}
	if !strings.Contains(out, "installed: demo") {
		t.Fatalf("install stdout: %s", out)
	}

	// Idempotent re-install.
	if code, _, e := runIntegration(t, "install", "demo"); code != 0 {
		t.Fatalf("reinstall: %d %s", code, e)
	}

	// List shows installed.
	_, listOut, _ := runIntegration(t, "list")
	if !strings.Contains(listOut, "demo") || !strings.Contains(listOut, "true") {
		t.Fatalf("list: %s", listOut)
	}

	// Hook exists.
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), "h.sh")); err != nil {
		t.Fatalf("hook missing: %v", err)
	}

	code, _, errOut = runIntegration(t, "uninstall", "demo")
	if code != 0 {
		t.Fatalf("uninstall: %d %s", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), "h.sh")); !os.IsNotExist(err) {
		t.Fatalf("hook still there")
	}
}

func TestIntegration_UnknownHarness(t *testing.T) {
	setupTestHarness(t)
	if code, _, _ := runIntegration(t, "install", "nope"); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
}
