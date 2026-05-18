package sidebar

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestPaletteHasAllRoles(t *testing.T) {
	v := reflect.ValueOf(DefaultPalette)
	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i).Interface().(lipgloss.AdaptiveColor)
		if f.Light == "" || f.Dark == "" {
			t.Errorf("field %s missing variant: %+v", typ.Field(i).Name, f)
		}
	}
}

func TestGradientRGB_Endpoints(t *testing.T) {
	for _, dark := range []bool{false, true} {
		c0 := gradientRGB(DefaultPalette.AccentA, DefaultPalette.AccentB, 0, dark)
		c1 := gradientRGB(DefaultPalette.AccentA, DefaultPalette.AccentB, 1, dark)
		wantA := pickVariant(DefaultPalette.AccentA, dark)
		wantB := pickVariant(DefaultPalette.AccentB, dark)
		if !strings.EqualFold(string(c0), wantA) {
			t.Errorf("dark=%v t=0 got %s want %s", dark, c0, wantA)
		}
		if !strings.EqualFold(string(c1), wantB) {
			t.Errorf("dark=%v t=1 got %s want %s", dark, c1, wantB)
		}
	}
}

func TestGradientRGB_Midpoint(t *testing.T) {
	for _, dark := range []bool{false, true} {
		ar, ag, ab := parseHex(pickVariant(DefaultPalette.AccentA, dark))
		br, bg, bb := parseHex(pickVariant(DefaultPalette.AccentB, dark))
		mid := gradientRGB(DefaultPalette.AccentA, DefaultPalette.AccentB, 0.5, dark)
		mr, mg, mb := parseHex(string(mid))
		check := func(name string, got, a, b uint8) {
			want := int(a) + (int(b)-int(a))/2
			diff := int(got) - want
			if diff < -1 || diff > 1 {
				t.Errorf("dark=%v %s got %d want ~%d (±1)", dark, name, got, want)
			}
		}
		check("r", mr, ar, br)
		check("g", mg, ag, bg)
		check("b", mb, ab, bb)
	}
}

func TestRenderLogo_PreservesLineCount(t *testing.T) {
	out := RenderLogo(Logo, DefaultPalette, true)
	if len(out) != len(Logo) {
		t.Fatalf("got %d lines want %d", len(out), len(Logo))
	}
	for i, ln := range out {
		got := len([]rune(stripANSI(ln)))
		want := len([]rune(Logo[i]))
		if got != want {
			t.Errorf("line %d visible runes %d want %d", i, got, want)
		}
	}
}

func TestRenderLogo_StrippedEqualsSource(t *testing.T) {
	out := RenderLogo(Logo, DefaultPalette, true)
	for i, ln := range out {
		if got := stripANSI(ln); got != Logo[i] {
			t.Errorf("line %d stripped %q != src %q", i, got, Logo[i])
		}
	}
}
