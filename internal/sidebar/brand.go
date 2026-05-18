package sidebar

// Logo is the ASCII-art wordmark rendered at the top of the sidebar.
//
// It is the stable extension point for future theming (PRD 002 §8):
// replacing the slice — at init time or via a future setter — changes
// the rendered logo without touching call sites. Keep entries the same
// visible width across lines.
var Logo = []string{
	"███▄███▄ ██ ██ ████▄ ▄█▀█▄",
	"██ ██ ██ ██ ██ ██ ▀▀ ██▄█▀",
	"██ ██ ██ ▀██▀█ ██    ▀█▄▄▄",
}

// Wordmark is the plain-text fallback used when the pane is too narrow
// to render Logo (PRD 002 §7.1).
const Wordmark = "mure"
