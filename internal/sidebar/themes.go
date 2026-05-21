package sidebar

import "github.com/charmbracelet/lipgloss"

// NamedTheme bundles a display name with a Palette. Themes is the
// catalog cycled by the in-sidebar picker (prefix-key `t`).
type NamedTheme struct {
	Name    string
	Palette Palette
}

// Themes is the ordered list of bundled themes. The first entry is the
// default palette used at sidebar startup.
var Themes = []NamedTheme{
	{
		Name: "Catppuccin Mocha",
		Palette: Palette{
			AccentA:     lipgloss.AdaptiveColor{Light: "#cba6f7", Dark: "#cba6f7"},
			AccentB:     lipgloss.AdaptiveColor{Light: "#f5c2e7", Dark: "#f5c2e7"},
			Working:     lipgloss.AdaptiveColor{Light: "#a6e3a1", Dark: "#a6e3a1"},
			Blocked:     lipgloss.AdaptiveColor{Light: "#f38ba8", Dark: "#f38ba8"},
			Idle:        lipgloss.AdaptiveColor{Light: "#9399b2", Dark: "#9399b2"},
			Dim:         lipgloss.AdaptiveColor{Light: "#9399b2", Dark: "#9399b2"},
			SelectionBG: lipgloss.AdaptiveColor{Light: "#313244", Dark: "#313244"},
			SelectionFG: lipgloss.AdaptiveColor{Light: "#cdd6f4", Dark: "#cdd6f4"},
			Background:  lipgloss.AdaptiveColor{Light: "#1d1d2e", Dark: "#1d1d2e"},
			Divider:     lipgloss.AdaptiveColor{Light: "#313244", Dark: "#313244"},
		},
	},
	{
		Name: "Catppuccin Latte",
		Palette: Palette{
			AccentA:     lipgloss.AdaptiveColor{Light: "#8839ef", Dark: "#8839ef"},
			AccentB:     lipgloss.AdaptiveColor{Light: "#ea76cb", Dark: "#ea76cb"},
			Working:     lipgloss.AdaptiveColor{Light: "#40a02b", Dark: "#40a02b"},
			Blocked:     lipgloss.AdaptiveColor{Light: "#d20f39", Dark: "#d20f39"},
			Idle:        lipgloss.AdaptiveColor{Light: "#6c6f85", Dark: "#6c6f85"},
			Dim:         lipgloss.AdaptiveColor{Light: "#6c6f85", Dark: "#6c6f85"},
			SelectionBG: lipgloss.AdaptiveColor{Light: "#dce0e8", Dark: "#dce0e8"},
			SelectionFG: lipgloss.AdaptiveColor{Light: "#4c4f69", Dark: "#4c4f69"},
			Background:  lipgloss.AdaptiveColor{Light: "#eff1f5", Dark: "#eff1f5"},
			Divider:     lipgloss.AdaptiveColor{Light: "#ccd0da", Dark: "#ccd0da"},
		},
	},
	{
		Name: "Tokyo Night",
		Palette: Palette{
			AccentA:     lipgloss.AdaptiveColor{Light: "#bb9af7", Dark: "#bb9af7"},
			AccentB:     lipgloss.AdaptiveColor{Light: "#f7768e", Dark: "#f7768e"},
			Working:     lipgloss.AdaptiveColor{Light: "#9ece6a", Dark: "#9ece6a"},
			Blocked:     lipgloss.AdaptiveColor{Light: "#f7768e", Dark: "#f7768e"},
			Idle:        lipgloss.AdaptiveColor{Light: "#565f89", Dark: "#565f89"},
			Dim:         lipgloss.AdaptiveColor{Light: "#565f89", Dark: "#565f89"},
			SelectionBG: lipgloss.AdaptiveColor{Light: "#283457", Dark: "#283457"},
			SelectionFG: lipgloss.AdaptiveColor{Light: "#c0caf5", Dark: "#c0caf5"},
			Background:  lipgloss.AdaptiveColor{Light: "#1a1b26", Dark: "#1a1b26"},
			Divider:     lipgloss.AdaptiveColor{Light: "#283457", Dark: "#283457"},
		},
	},
	{
		Name: "Gruvbox Dark",
		Palette: Palette{
			AccentA:     lipgloss.AdaptiveColor{Light: "#d3869b", Dark: "#d3869b"},
			AccentB:     lipgloss.AdaptiveColor{Light: "#fe8019", Dark: "#fe8019"},
			Working:     lipgloss.AdaptiveColor{Light: "#b8bb26", Dark: "#b8bb26"},
			Blocked:     lipgloss.AdaptiveColor{Light: "#fb4934", Dark: "#fb4934"},
			Idle:        lipgloss.AdaptiveColor{Light: "#928374", Dark: "#928374"},
			Dim:         lipgloss.AdaptiveColor{Light: "#928374", Dark: "#928374"},
			SelectionBG: lipgloss.AdaptiveColor{Light: "#3c3836", Dark: "#3c3836"},
			SelectionFG: lipgloss.AdaptiveColor{Light: "#ebdbb2", Dark: "#ebdbb2"},
			Background:  lipgloss.AdaptiveColor{Light: "#282828", Dark: "#282828"},
			Divider:     lipgloss.AdaptiveColor{Light: "#3c3836", Dark: "#3c3836"},
		},
	},
}
