package harnesses

import "testing"

func TestMarkers_RoundTrip(t *testing.T) {
	user := "alpha\nbeta\n"
	out := ReplaceOrAppendBlock(user, "pi", "x=1")
	want := "alpha\nbeta\n# >>> mure:pi >>>\nx=1\n# <<< mure:pi <<<\n"
	if out != want {
		t.Fatalf("append:\n got: %q\nwant: %q", out, want)
	}
	// Idempotent replace with new body.
	out2 := ReplaceOrAppendBlock(out, "pi", "y=2")
	want2 := "alpha\nbeta\n# >>> mure:pi >>>\ny=2\n# <<< mure:pi <<<\n"
	if out2 != want2 {
		t.Fatalf("replace:\n got: %q\nwant: %q", out2, want2)
	}
	// Strip restores surrounding content.
	stripped := StripBlock(out2, "pi")
	if stripped != user {
		t.Fatalf("strip:\n got: %q\nwant: %q", stripped, user)
	}
}

func TestMarkers_EmptyFile(t *testing.T) {
	out := ReplaceOrAppendBlock("", "h", "body")
	if out != "# >>> mure:h >>>\nbody\n# <<< mure:h <<<\n" {
		t.Fatalf("empty: %q", out)
	}
}
