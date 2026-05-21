package harnesses

import (
	"strings"
	"testing"
)

const validTOML = `
name = "pi"
display = "Pi"
command = "pi"
task_arg = "positional"

[capabilities]
spawn = true
status = true
result = true
subtools = false

[install.skill]
path = "~/.config/pi/skill.md"
merge = "append"

[[install.hooks]]
src = "hooks/on-tool-start.sh"
dst = "~/.config/pi/hooks/on-tool-start.sh"
mode = "0755"
`

func TestDecodeManifest_Valid(t *testing.T) {
	m, err := DecodeManifest([]byte(validTOML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Name != "pi" || m.Command != "pi" {
		t.Fatalf("bad fields: %+v", m)
	}
	if !m.Capabilities.Status || m.Capabilities.Subtools {
		t.Fatalf("bad caps: %+v", m.Capabilities)
	}
	if len(m.Install.Hooks) != 1 || m.Install.Hooks[0].Mode != "0755" {
		t.Fatalf("bad hooks: %+v", m.Install.Hooks)
	}
}

func TestDecodeManifest_UnknownKey(t *testing.T) {
	bad := validTOML + "\nextra_unknown_key = 1\n"
	_, err := DecodeManifest([]byte(bad))
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestDecodeManifest_MissingName(t *testing.T) {
	_, err := DecodeManifest([]byte(`command = "x"`))
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("expected name error, got %v", err)
	}
}

func TestDecodeManifest_MissingCommand(t *testing.T) {
	_, err := DecodeManifest([]byte(`name = "x"`))
	if err == nil || !strings.Contains(err.Error(), "command") {
		t.Fatalf("expected command error, got %v", err)
	}
}

func TestDecodeManifest_InvalidTaskArg(t *testing.T) {
	src := `
name = "x"
command = "y"
task_arg = "bogus"
`
	_, err := DecodeManifest([]byte(src))
	if err == nil || !strings.Contains(err.Error(), "task_arg") {
		t.Fatalf("expected task_arg error, got %v", err)
	}
}

func TestDecodeManifest_InvalidMerge(t *testing.T) {
	src := `
name = "x"
command = "y"

[install.skill]
path = "~/x"
merge = "bogus"
`
	_, err := DecodeManifest([]byte(src))
	if err == nil || !strings.Contains(err.Error(), "merge") {
		t.Fatalf("expected merge error, got %v", err)
	}
}

func TestParseTaskArg(t *testing.T) {
	cases := []struct {
		in   string
		kind TaskArgKind
		flag string
	}{
		{"", TaskArgNone, ""},
		{"none", TaskArgNone, ""},
		{"positional", TaskArgPositional, ""},
		{"stdin", TaskArgStdin, ""},
		{"flag:--prompt", TaskArgFlag, "--prompt"},
	}
	for _, c := range cases {
		got, err := ParseTaskArg(c.in)
		if err != nil {
			t.Errorf("%q: unexpected error %v", c.in, err)
			continue
		}
		if got.Kind != c.kind || got.Flag != c.flag {
			t.Errorf("%q: got %+v", c.in, got)
		}
	}
	for _, bad := range []string{"weird", "flag:", "flag"} {
		if _, err := ParseTaskArg(bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}
