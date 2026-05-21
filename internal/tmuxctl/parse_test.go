package tmuxctl

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"
)

func TestParseLineTable(t *testing.T) {
	cases := []struct {
		name string
		line string
		want Event
	}{
		{"begin", "%begin 1700000000 1 0", Event{Kind: EventBegin, Time: "1700000000", Number: "1", Flags: "0"}},
		{"end", "%end 1700000000 1 0", Event{Kind: EventEnd, Time: "1700000000", Number: "1", Flags: "0"}},
		{"error", "%error 1700000000 2 0", Event{Kind: EventError, Time: "1700000000", Number: "2", Flags: "0"}},
		{"window-add", "%window-add @5", Event{Kind: EventWindowAdd, WindowID: "@5"}},
		{"window-close", "%window-close @5", Event{Kind: EventWindowClose, WindowID: "@5"}},
		{"session-window-changed", "%session-window-changed $0 @3", Event{Kind: EventSessionWindowChanged, SessionID: "$0", WindowID: "@3"}},
		{"layout-change", "%layout-change @3 b25d,80x24,0,0,0 1", Event{Kind: EventLayoutChange, WindowID: "@3", Layout: "b25d,80x24,0,0,0 1"}},
		{"output", "%output %41 hello\\040world", Event{Kind: EventOutput, PaneID: "%41", Output: "hello\\040world"}},
		{"unknown", "%mystery foo", Event{Kind: EventUnknown}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseLine(tc.line)
			if !ok {
				t.Fatalf("ParseLine returned ok=false")
			}
			tc.want.Raw = tc.line
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseLineNonControl(t *testing.T) {
	if _, ok := ParseLine("not a control line"); ok {
		t.Fatal("expected ok=false for non-% line")
	}
}

func TestParseFixtureFile(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "events.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	wantKinds := []EventKind{
		EventBegin, EventEnd, EventError,
		EventWindowAdd, EventWindowClose,
		EventSessionWindowChanged, EventLayoutChange, EventOutput,
	}
	s := bufio.NewScanner(f)
	i := 0
	for s.Scan() {
		ev, ok := ParseLine(s.Text())
		if !ok {
			t.Fatalf("line %d: not parsed", i)
		}
		if ev.Kind != wantKinds[i] {
			t.Fatalf("line %d: kind=%d want=%d", i, ev.Kind, wantKinds[i])
		}
		i++
	}
	if i != len(wantKinds) {
		t.Fatalf("read %d lines, want %d", i, len(wantKinds))
	}
}
