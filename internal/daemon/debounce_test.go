package daemon

import (
	"sync"
	"testing"
	"time"
)

type recorder struct {
	mu  sync.Mutex
	got []string
}

func (r *recorder) cb(prefix string) func(string) {
	return func(id string) {
		r.mu.Lock()
		r.got = append(r.got, prefix+":"+id)
		r.mu.Unlock()
	}
}

func (r *recorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.got))
	copy(out, r.got)
	return out
}

func TestDebounceEPIPEAloneFiresDisconnected(t *testing.T) {
	rec := &recorder{}
	d := NewDebouncer(50*time.Millisecond, rec.cb("err"), rec.cb("dis"))
	d.OnEPIPE("a")
	time.Sleep(150 * time.Millisecond)
	got := rec.snapshot()
	if len(got) != 1 || got[0] != "dis:a" {
		t.Fatalf("want [dis:a], got %v", got)
	}
}

func TestDebounceEPIPEThenPaneDiedFiresErrored(t *testing.T) {
	rec := &recorder{}
	d := NewDebouncer(200*time.Millisecond, rec.cb("err"), rec.cb("dis"))
	d.OnEPIPE("a")
	time.Sleep(50 * time.Millisecond)
	d.OnPaneDied("a")
	time.Sleep(300 * time.Millisecond)
	got := rec.snapshot()
	if len(got) != 1 || got[0] != "err:a" {
		t.Fatalf("want [err:a], got %v", got)
	}
}

func TestDebounceStretchExtendsWindow(t *testing.T) {
	rec := &recorder{}
	d := NewDebouncer(50*time.Millisecond, rec.cb("err"), rec.cb("dis"))
	d.OnEPIPE("a")
	d.Stretch("a", 200*time.Millisecond)
	time.Sleep(120 * time.Millisecond)
	if len(rec.snapshot()) != 0 {
		t.Fatalf("expected no callbacks yet, got %v", rec.snapshot())
	}
	time.Sleep(200 * time.Millisecond)
	got := rec.snapshot()
	if len(got) != 1 || got[0] != "dis:a" {
		t.Fatalf("want [dis:a] after stretch, got %v", got)
	}
}
