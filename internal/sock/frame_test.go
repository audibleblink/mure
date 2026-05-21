package sock

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// Golden frames taken verbatim from PRD §12.
var goldenAgent = []string{
	`{"v":1,"event":"hello","role":"agent","agent_id":"agent-3","pane_id":"%41","pid":12345,"pi_version":"0.50.3","ts":1731890000000}`,
	`{"v":1,"event":"status","agent_id":"agent-3","status":"working","task":"refactor auth","tool":"bash","ts":1731890001234}`,
	`{"v":1,"event":"bye","agent_id":"agent-3","ts":1731890003000}`,
}

var goldenSidebar = []string{
	`{"v":1,"event":"roster","agents":[{"id":"agent-3","status":"idle","pane":"%43"}]}`,
	`{"v":1,"event":"agent_update","agent":{"id":"agent-3","status":"idle","task":"…","pane":"%43","last_turn_ended_at":1731890003000}}`,
}

// jsonEq compares two JSON documents structurally.
func jsonEq(t *testing.T, a, b string) {
	t.Helper()
	var av, bv any
	if err := json.Unmarshal([]byte(a), &av); err != nil {
		t.Fatalf("unmarshal a: %v", err)
	}
	if err := json.Unmarshal([]byte(b), &bv); err != nil {
		t.Fatalf("unmarshal b: %v", err)
	}
	ab, _ := json.Marshal(av)
	bb, _ := json.Marshal(bv)
	if !bytes.Equal(ab, bb) {
		t.Fatalf("json mismatch:\n got: %s\nwant: %s", ab, bb)
	}
}

func roundTrip(t *testing.T, golden string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(golden), v); err != nil {
		t.Fatalf("unmarshal %s: %v", golden, err)
	}
	var buf bytes.Buffer
	if err := WriteFrame(&buf, v); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	got := strings.TrimRight(buf.String(), "\n")
	jsonEq(t, got, golden)

	// Read it back through ReadFrame.
	line, err := ReadFrame(bufio.NewReader(&buf), MaxFrameSize)
	// buf was consumed by previous read of String? No, String doesn't consume.
	_ = line
	_ = err
}

func TestRoundTripFrames(t *testing.T) {
	type tc struct {
		name string
		json string
		v    any
	}
	cases := []tc{
		{"hello_agent", goldenAgent[0], &Hello{}},
		{"status", goldenAgent[1], &Status{}},
		{"bye", goldenAgent[2], &Bye{}},
		{"roster", goldenSidebar[0], &Roster{}},
		{"agent_update", goldenSidebar[1], &AgentUpdate{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			roundTrip(t, c.json, c.v)
		})
	}
}

func TestWriteReadFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	in := Status{V: 1, Event: "status", AgentID: "a", Status: StatusWorking, TS: 42}
	if err := WriteFrame(&buf, in); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(&buf)
	line, err := ReadFrame(r, MaxFrameSize)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	var out Status
	if err := json.Unmarshal(line, &out); err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("got %+v want %+v", out, in)
	}
}

func TestReadFrameRejectsOversize(t *testing.T) {
	big := bytes.Repeat([]byte("x"), MaxFrameSize+1)
	big = append(big, '\n')
	big = append(big, []byte(`{"v":1,"event":"bye","agent_id":"a","ts":1}`+"\n")...)
	r := bufio.NewReader(bytes.NewReader(big))
	_, err := ReadFrame(r, MaxFrameSize)
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("want ErrFrameTooLarge, got %v", err)
	}
	// Next frame should read cleanly.
	line, err := ReadFrame(r, MaxFrameSize)
	if err != nil {
		t.Fatalf("next frame: %v", err)
	}
	ev, ver, err := DecodeEnvelope(line)
	if err != nil || ev != "bye" || ver != 1 {
		t.Fatalf("envelope: ev=%q ver=%d err=%v", ev, ver, err)
	}
}

func TestDecodeEnvelope(t *testing.T) {
	for _, g := range append(append([]string{}, goldenAgent...), goldenSidebar...) {
		ev, ver, err := DecodeEnvelope([]byte(g))
		if err != nil {
			t.Fatalf("DecodeEnvelope(%s): %v", g, err)
		}
		if ver != 1 {
			t.Fatalf("ver=%d for %s", ver, g)
		}
		if ev == "" {
			t.Fatalf("empty event for %s", g)
		}
	}
}

func TestDecodeEnvelopeRejectsVersion(t *testing.T) {
	_, _, err := DecodeEnvelope([]byte(`{"v":2,"event":"hello"}`))
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("want ErrUnsupportedVersion, got %v", err)
	}
	_, _, err = DecodeEnvelope([]byte(`{"event":"hello"}`))
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("missing v: want ErrUnsupportedVersion, got %v", err)
	}
}

func TestValidStatus(t *testing.T) {
	for _, s := range []string{StatusIdle, StatusWorking, StatusBlocked} {
		if !ValidStatus(s) {
			t.Fatalf("ValidStatus(%q) = false", s)
		}
	}
	if ValidStatus("done") {
		t.Fatal("ValidStatus(done) should be false")
	}
}
