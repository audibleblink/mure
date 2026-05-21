package sidebar

import (
	"fmt"
	"strconv"

	"github.com/charmbracelet/lipgloss"
)

// Palette holds every adaptive color role used by the sidebar (PRD 002 §6.1).
type Palette struct {
	AccentA      lipgloss.AdaptiveColor
	AccentB      lipgloss.AdaptiveColor
	Working lipgloss.AdaptiveColor
	Blocked lipgloss.AdaptiveColor
	Idle    lipgloss.AdaptiveColor
	Dim          lipgloss.AdaptiveColor
	SelectionBG  lipgloss.AdaptiveColor
	SelectionFG  lipgloss.AdaptiveColor
	Background   lipgloss.AdaptiveColor
	Divider      lipgloss.AdaptiveColor
}

// DefaultPalette is the first bundled theme (see themes.go). The sidebar
// boots with this palette; users can cycle through Themes via the in-pane
// picker (prefix-key `t`).
var DefaultPalette = Themes[0].Palette

var active = DefaultPalette

// ActivePalette returns the palette currently in use. PRD 002 §8: this
// indirection lets a future setter swap palettes without touching call sites.
func ActivePalette() Palette { return active }

// parseHex parses a "#rrggbb" string into RGB components.
func parseHex(s string) (uint8, uint8, uint8) {
	if len(s) == 7 && s[0] == '#' {
		s = s[1:]
	}
	r, _ := strconv.ParseUint(s[0:2], 16, 8)
	g, _ := strconv.ParseUint(s[2:4], 16, 8)
	b, _ := strconv.ParseUint(s[4:6], 16, 8)
	return uint8(r), uint8(g), uint8(b)
}

// pickVariant returns the active light/dark hex for an adaptive color.
func pickVariant(c lipgloss.AdaptiveColor, dark bool) string {
	if dark {
		return c.Dark
	}
	return c.Light
}

// gradientRGB linearly blends between the chosen variants of a and b at
// position t in [0,1] and returns the result as a #rrggbb lipgloss.Color.
func gradientRGB(a, b lipgloss.AdaptiveColor, t float64, dark bool) lipgloss.Color {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	ar, ag, ab := parseHex(pickVariant(a, dark))
	br, bg, bb := parseHex(pickVariant(b, dark))
	lerp := func(x, y uint8) uint8 {
		return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5)
	}
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", lerp(ar, br), lerp(ag, bg), lerp(ab, bb)))
}

// RenderLogo applies a per-rune linear gradient between p.AccentA and
// p.AccentB across the visible (non-space) runes of all lines joined,
// so the gradient spans the whole logo rather than each line in
// isolation. Every rune (spaces included) carries p.Background so the
// logo sits cleanly on a filled panel background.
func RenderLogo(lines []string, p Palette, dark bool) []string {
	// Count total visible runes (non-space) across all lines for t denom.
	total := 0
	for _, ln := range lines {
		for _, r := range ln {
			if r != ' ' {
				total++
			}
		}
	}
	bg := lipgloss.NewStyle().Background(p.Background)
	out := make([]string, len(lines))
	idx := 0
	denom := float64(total - 1)
	for i, ln := range lines {
		var buf string
		for _, r := range ln {
			if r == ' ' {
				buf += bg.Render(" ")
				continue
			}
			var t float64
			if denom <= 0 {
				t = 0
			} else {
				t = float64(idx) / denom
			}
			col := gradientRGB(p.AccentA, p.AccentB, t, dark)
			buf += lipgloss.NewStyle().Foreground(col).Background(p.Background).Render(string(r))
			idx++
		}
		out[i] = buf
	}
	return out
}
