package sidebar

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func init() {
	// Force a deterministic color profile so SGR sequences are emitted
	// in tests regardless of TTY detection.
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)
}
