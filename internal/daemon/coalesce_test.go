package daemon

import (
	"testing"
	"time"
)

func TestCoalesceCollapsesFiveWrites(t *testing.T) {
	c := NewCoalescer(80 * time.Millisecond)
	defer c.Close()
	for i, v := range []string{"a", "b", "c", "d", "e"} {
		c.Submit("%1", "@mure-status", v)
		if i < 4 {
			time.Sleep(10 * time.Millisecond)
		}
	}
	select {
	case w := <-c.Out():
		if w.Value != "e" {
			t.Fatalf("want last value 'e', got %q", w.Value)
		}
		if w.PaneID != "%1" || w.Option != "@mure-status" {
			t.Fatalf("unexpected write: %+v", w)
		}
	case <-time.After(time.Second):
		t.Fatal("no emission")
	}
	// Should be no further emission.
	select {
	case w := <-c.Out():
		t.Fatalf("unexpected second emission: %+v", w)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestCoalesceDistinctKeysIndependent(t *testing.T) {
	c := NewCoalescer(40 * time.Millisecond)
	defer c.Close()
	c.Submit("%1", "@mure-status", "working")
	c.Submit("%2", "@mure-status", "idle")
	seen := map[string]string{}
	for i := 0; i < 2; i++ {
		select {
		case w := <-c.Out():
			seen[w.PaneID] = w.Value
		case <-time.After(time.Second):
			t.Fatalf("missing emission %d", i)
		}
	}
	if seen["%1"] != "working" || seen["%2"] != "idle" {
		t.Fatalf("got %v", seen)
	}
}
