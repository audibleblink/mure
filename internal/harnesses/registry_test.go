package harnesses

import (
	"testing"
	"testing/fstest"
)

func TestLoad_SortsAndGets(t *testing.T) {
	mfs := fstest.MapFS{
		"zeta/manifest.toml":  {Data: []byte(`name = "zeta"` + "\n" + `command = "z"` + "\n")},
		"alpha/manifest.toml": {Data: []byte(`name = "alpha"` + "\n" + `command = "a"` + "\n")},
		"notdir.txt":          {Data: []byte("ignored")},
		"nomanifest/other":    {Data: []byte("ignored")},
	}
	ms, err := Load(mfs)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(ms) != 2 || ms[0].Name != "alpha" || ms[1].Name != "zeta" {
		t.Fatalf("bad result: %+v", ms)
	}
	if _, ok := Get(ms, "alpha"); !ok {
		t.Fatal("Get(alpha) missed")
	}
	if _, ok := Get(ms, "missing"); ok {
		t.Fatal("Get(missing) should miss")
	}
}

func TestLoad_AggregatesErrors(t *testing.T) {
	mfs := fstest.MapFS{
		"good/manifest.toml": {Data: []byte(`name = "good"` + "\n" + `command = "g"` + "\n")},
		"bad/manifest.toml":  {Data: []byte(`command = "x"` + "\n")}, // missing name
	}
	ms, err := Load(mfs)
	if err == nil {
		t.Fatal("expected aggregate error")
	}
	if len(ms) != 1 || ms[0].Name != "good" {
		t.Fatalf("bad ms: %+v", ms)
	}
}
