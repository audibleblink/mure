package sidebar

import "regexp"

// ansiRE matches CSI SGR sequences emitted by lipgloss.
var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripANSI returns s with all ANSI SGR escape sequences removed.
func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }
