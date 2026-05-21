package harnesses

import (
	"path/filepath"
	"testing"
)

func TestStateDir_XDG(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/x/y")
	d, err := StateDir()
	if err != nil {
		t.Fatal(err)
	}
	if d != filepath.Join("/x/y", "mure", "integrations") {
		t.Fatalf("xdg: %s", d)
	}
}

func TestStateDir_Fallback(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/home/u")
	d, err := StateDir()
	if err != nil {
		t.Fatal(err)
	}
	if d != "/home/u/.local/state/mure/integrations" {
		t.Fatalf("fallback: %s", d)
	}
}

func TestState_WriteReadClear(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)
	r := Receipt{Harness: "h", Files: []FileReceipt{{Dst: "/a", SHA: "abc", Merge: "replace"}}}
	if err := WriteState("h", r); err != nil {
		t.Fatal(err)
	}
	got, ok, err := ReadState("h")
	if err != nil || !ok {
		t.Fatalf("read: %v %v", ok, err)
	}
	if got.Harness != "h" || len(got.Files) != 1 || got.Files[0].SHA != "abc" {
		t.Fatalf("got %+v", got)
	}
	if err := ClearState("h"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := ReadState("h"); ok {
		t.Fatal("expected cleared")
	}
}
