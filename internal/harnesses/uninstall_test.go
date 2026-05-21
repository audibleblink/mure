package harnesses

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestInstallUninstall_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))

	skillPath := filepath.Join(tmp, "skill.md")
	if err := os.WriteFile(skillPath, []byte("user\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := Manifest{
		Name: "demo", Command: "demo",
		Install: Install{
			Skill: Skill{Path: "~/skill.md", Merge: "append"},
			Files: []File{{Src: "h.sh", Dst: "~/h.sh", Mode: "0755"}},
		},
	}
	fs := fstest.MapFS{
		"demo/skill.md": {Data: []byte("body")},
		"demo/h.sh":     {Data: []byte("#!/bin/sh\n")},
	}
	ops, err := BuildPlan(m, fs)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Apply("demo", ops)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteState("demo", r); err != nil {
		t.Fatal(err)
	}

	if warns := Uninstall(r); len(warns) != 0 {
		t.Fatalf("warns: %v", warns)
	}
	if err := ClearState("demo"); err != nil {
		t.Fatal(err)
	}

	b, _ := os.ReadFile(skillPath)
	if string(b) != "user\n" {
		t.Fatalf("skill not restored: %q", b)
	}
	if _, err := os.Stat(filepath.Join(tmp, "h.sh")); !os.IsNotExist(err) {
		t.Fatalf("hook still exists: %v", err)
	}
	if _, ok, _ := ReadState("demo"); ok {
		t.Fatal("state not cleared")
	}
}

func TestUninstall_ModifiedReplaceFile_IsWarned(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	m := Manifest{
		Name: "demo", Command: "demo",
		Install: Install{Files: []File{{Src: "h.sh", Dst: "~/h.sh", Mode: "0644"}}},
	}
	fs := fstest.MapFS{"demo/h.sh": {Data: []byte("orig")}}
	ops, _ := BuildPlan(m, fs)
	r, _ := Apply("demo", ops)
	// User modifies it.
	if err := os.WriteFile(filepath.Join(tmp, "h.sh"), []byte("modified"), 0o644); err != nil {
		t.Fatal(err)
	}
	warns := Uninstall(r)
	if len(warns) != 1 {
		t.Fatalf("want 1 warn, got %v", warns)
	}
	b, _ := os.ReadFile(filepath.Join(tmp, "h.sh"))
	if string(b) != "modified" {
		t.Fatalf("file removed despite mod")
	}
}
