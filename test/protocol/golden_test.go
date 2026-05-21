// Package protocol_test verifies that the Go encoder and the TS encoder
// produce byte-identical NDJSON for every fixture frame. The TS side is
// covered by pi-mure/test/cross.test.ts; this file ensures the Go side
// also round-trips each fixture, and that both sides share the *same*
// fixture file (one copy under pi-mure/test/fixtures/, mirrored via
// `make sync-piext` into internal/piext/assets/).
package protocol_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/audibleblink/mure/internal/sock"
)

func fixturePath(t *testing.T) string {
	t.Helper()
	// Walk up to repo root, then pi-mure/test/fixtures/frames.json.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for i := 0; i < 6; i++ {
		p := filepath.Join(dir, "pi-mure", "test", "fixtures", "frames.json")
		if _, err := os.Stat(p); err == nil {
			return p
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("frames.json not found")
	return ""
}

// fixtures preserves key order by storing each frame as a raw JSON message.
type fixtures map[string]json.RawMessage

func loadFixtures(t *testing.T) fixtures {
	t.Helper()
	b, err := os.ReadFile(fixturePath(t))
	if err != nil {
		t.Fatal(err)
	}
	var f fixtures
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

// frameType returns a fresh typed struct pointer keyed by event/role.
func frameType(name string) any {
	switch name {
	case "hello_agent":
		return &sock.Hello{}
	case "status":
		return &sock.Status{}
	case "bye":
		return &sock.Bye{}
	case "roster":
		return &sock.Roster{}
	case "agent_update":
		return &sock.AgentUpdate{}
	}
	return nil
}

func TestGoEncodesEveryFixtureByteIdentically(t *testing.T) {
	fx := loadFixtures(t)
	for name, raw := range fx {
		v := frameType(name)
		if v == nil {
			t.Errorf("%s: no Go type registered", name)
			continue
		}
		// Trim incidental whitespace from RawMessage.
		line := strings.TrimSpace(string(raw))
		if err := json.Unmarshal([]byte(line), v); err != nil {
			t.Errorf("%s: decode: %v", name, err)
			continue
		}
		out, err := json.Marshal(v)
		if err != nil {
			t.Errorf("%s: encode: %v", name, err)
			continue
		}
		if !bytes.Equal(out, []byte(line)) {
			t.Errorf("%s: byte mismatch\n got: %s\nwant: %s", name, out, line)
		}
	}
}
