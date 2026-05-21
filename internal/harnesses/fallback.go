// Package harnesses — fallback.go implements capture-pane based status &
// result fallback for harnesses whose manifests report capabilities.status
// or capabilities.result == false. Daemons poll panes via these helpers.
package harnesses

import (
	"strings"
	"time"
)

// DefaultIdleWindow is the spacing between capture-pane samples used to
// decide idle vs working when a harness lacks first-class status reporting.
const DefaultIdleWindow = 3 * time.Second

// DefaultResultLines is the number of trailing capture-pane lines returned
// as a degraded result.
const DefaultResultLines = 200

// CaptureRunner returns the current capture-pane output for the given pane.
// In production this wraps `tmux capture-pane -p -t <pane>`; tests inject a fake.
type CaptureRunner func(paneID string) (string, error)

// Sleeper is a sleep seam (defaults to time.Sleep) for tests.
type Sleeper func(time.Duration)

// ClassifyByCapture samples paneID twice, separated by window, and reports
// "idle" if the two captures are byte-identical, else "working". It also
// returns the most recent capture so callers can persist it for result
// fallback.
func ClassifyByCapture(run CaptureRunner, sleep Sleeper, paneID string, window time.Duration) (status string, last string, err error) {
	if sleep == nil {
		sleep = time.Sleep
	}
	first, err := run(paneID)
	if err != nil {
		return "", "", err
	}
	sleep(window)
	second, err := run(paneID)
	if err != nil {
		return "", "", err
	}
	if first == second {
		return "idle", second, nil
	}
	return "working", second, nil
}

// LastLines returns the trailing n newline-separated lines of buf, joined
// with "\n". Used to surface a "degraded" result payload to mure wait.
func LastLines(buf string, n int) string {
	if n <= 0 || buf == "" {
		return ""
	}
	lines := strings.Split(strings.TrimRight(buf, "\n"), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
