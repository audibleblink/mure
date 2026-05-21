package harnesses

import "testing"

// TestEmbeddedManifestsAllDecode iterates every harness in the shipped
// embedded FS and asserts the manifest decodes strictly. This is the CI
// guardrail for new harnesses landing under harnesses/.
func TestEmbeddedManifestsAllDecode(t *testing.T) {
	ms, err := Load(FS())
	if err != nil {
		t.Fatalf("load embedded harnesses: %v", err)
	}
	if len(ms) == 0 {
		t.Fatal("no embedded harnesses found")
	}
	want := map[string]bool{"pi": false, "claude": false, "opencode": false}
	for _, m := range ms {
		if _, ok := want[m.Name]; ok {
			want[m.Name] = true
		}
		// BuildPlan must succeed against the embedded FS for every harness.
		if _, err := BuildPlan(m, FS()); err != nil {
			t.Errorf("%s: BuildPlan: %v", m.Name, err)
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("expected embedded harness %q", name)
		}
	}
}
