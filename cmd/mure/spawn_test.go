package main

import (
	"errors"
	"strings"
	"testing"
)

func TestResolveHarness_Precedence(t *testing.T) {
	noTmux := func(args ...string) (string, error) { return "", errors.New("no tmux") }
	yesSession := func(args ...string) (string, error) {
		if len(args) >= 2 && args[0] == "show-option" && args[1] == "-qv" {
			return "from-session", nil
		}
		return "", errors.New("nope")
	}
	yesGlobal := func(args ...string) (string, error) {
		if len(args) >= 2 && args[0] == "show-option" && args[1] == "-gqv" {
			return "from-global", nil
		}
		return "", errors.New("nope")
	}

	cases := []struct {
		name string
		flag string
		env  string
		run  tmuxRunner
		want string
		err  bool
	}{
		{"flag wins", "fl", "ev", yesSession, "fl", false},
		{"env when no flag", "", "ev", yesSession, "ev", false},
		{"session when no env", "", "", yesSession, "from-session", false},
		{"global when no session", "", "", yesGlobal, "from-global", false},
		{"none → error", "", "", noTmux, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := resolveHarness(c.flag, c.env, c.run)
			if c.err {
				if err == nil {
					t.Fatal("want error")
				}
				msg := err.Error()
				for _, slot := range []string{"--harness", "MURE_HARNESS", "session", "global"} {
					if !strings.Contains(msg, slot) {
						t.Errorf("error %q missing slot %q", msg, slot)
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}
