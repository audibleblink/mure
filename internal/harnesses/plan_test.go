package harnesses

import (
	"path/filepath"
	"testing"
)

func TestExpandPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cases := []struct {
		name string
		env  map[string]string
		in   string
		want string
	}{
		{"tilde", nil, "~/foo", filepath.Join(home, "foo")},
		{"plain", nil, "/abs/path", "/abs/path"},
		{"env_set", map[string]string{"X": "/opt/x"}, "${X}/ext", "/opt/x/ext"},
		{"env_unset_default_literal", nil, "${MISSING:-/d}/e", "/d/e"},
		{"env_unset_default_tilde", nil, "${MISSING:-~/p}/e", filepath.Join(home, "p", "e")},
		{"env_set_overrides_default", map[string]string{"X": "/o"}, "${X:-/d}/e", "/o/e"},
		{"env_empty_uses_default", map[string]string{"X": ""}, "${X:-/d}/e", "/d/e"},
		{"bare_dollar", map[string]string{"X": "/o"}, "$X/e", "/o/e"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			got, err := expandPath(c.in)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}
