package harnesses

import (
	"strings"
	"testing"
	"time"
)

func TestClassifyByCapture_Idle(t *testing.T) {
	runs := 0
	run := func(string) (string, error) { runs++; return "same\n", nil }
	slept := time.Duration(0)
	sleep := func(d time.Duration) { slept += d }
	st, last, err := ClassifyByCapture(run, sleep, "%1", 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if st != "idle" {
		t.Errorf("status=%q, want idle", st)
	}
	if last != "same\n" {
		t.Errorf("last=%q", last)
	}
	if runs != 2 || slept != 3*time.Second {
		t.Errorf("runs=%d slept=%v", runs, slept)
	}
}

func TestClassifyByCapture_Working(t *testing.T) {
	outs := []string{"a", "b"}
	i := 0
	run := func(string) (string, error) { o := outs[i]; i++; return o, nil }
	st, _, err := ClassifyByCapture(run, func(time.Duration) {}, "%1", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if st != "working" {
		t.Errorf("status=%q, want working", st)
	}
}

func TestLastLines(t *testing.T) {
	buf := "1\n2\n3\n4\n5\n"
	got := LastLines(buf, 3)
	want := "3\n4\n5"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
	if LastLines("", 5) != "" {
		t.Error("empty buf should return empty")
	}
	if LastLines("only\n", 10) != "only" {
		t.Error("fewer lines than n")
	}
	if !strings.Contains(LastLines(buf, 100), "1\n2") {
		t.Error("all lines kept when n exceeds count")
	}
}
