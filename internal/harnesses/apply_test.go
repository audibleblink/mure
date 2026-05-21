package harnesses

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func sampleManifest(t *testing.T, tmp string) (Manifest, fstest.MapFS) {
	t.Helper()
	t.Setenv("HOME", tmp)
	m := Manifest{
		Name:    "demo",
		Command: "demo",
		Install: Install{
			Skill: Skill{Path: "~/.config/demo/skill.md", Merge: "append"},
			Files: []File{
				{Src: "hooks/start.sh", Dst: "~/.config/demo/hooks/start.sh", Mode: "0755"},
				{Src: "data.txt", Dst: "~/.config/demo/data.txt", Mode: "0644"},
			},
		},
	}
	fs := fstest.MapFS{
		"demo/SKILL.md":      {Data: []byte("skill-body")},
		"demo/hooks/start.sh": {Data: []byte("#!/bin/sh\necho hi\n")},
		"demo/data.txt":      {Data: []byte("data1\n")},
	}
	return m, fs
}

func TestApply_AllMergeModes_AndIdempotent(t *testing.T) {
	tmp := t.TempDir()
	m, fs := sampleManifest(t, tmp)

	// Pre-create skill file with prior user content to test append preservation.
	skillPath := filepath.Join(tmp, ".config/demo/skill.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, []byte("user-pre\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ops, err := BuildPlan(m, fs)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	r1, err := Apply("demo", ops)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// Skill file must still contain user content and a marker block.
	b, _ := os.ReadFile(skillPath)
	if !contains(b, "user-pre") || !contains(b, "# >>> mure:demo >>>") || !contains(b, "skill-body") {
		t.Fatalf("skill content wrong: %q", b)
	}
	// Hook installed with mode 0755.
	hookPath := filepath.Join(tmp, ".config/demo/hooks/start.sh")
	st, err := os.Stat(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o755 {
		t.Fatalf("hook mode = %v", st.Mode().Perm())
	}

	// Apply again, idempotent.
	r2, err := Apply("demo", ops)
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := os.ReadFile(skillPath)
	if string(b) != string(b2) {
		t.Fatalf("not idempotent:\n%s\n---\n%s", b, b2)
	}
	if len(r1.Files) != len(r2.Files) {
		t.Fatalf("receipt mismatch")
	}
}

func TestApply_CreateIfMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	m := Manifest{
		Name: "x", Command: "x",
		Install: Install{Skill: Skill{Path: "~/file", Merge: "create-if-missing"}},
	}
	fs := fstest.MapFS{"x/SKILL.md": {Data: []byte("new")}}
	ops, err := BuildPlan(m, fs)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(tmp, "file")
	if err := os.WriteFile(p, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply("x", ops); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "existing" {
		t.Fatalf("create-if-missing overwrote: %q", b)
	}
}

func contains(b []byte, s string) bool {
	return len(b) >= len(s) && string(b) != "" && bytesIndex(b, []byte(s)) >= 0
}

func bytesIndex(haystack, needle []byte) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if string(haystack[i:i+len(needle)]) == string(needle) {
			return i
		}
	}
	return -1
}
