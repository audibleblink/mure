package tmuxctl

import "strings"

// EventKind identifies the type of a tmux control-mode notification.
type EventKind int

const (
	// EventUnknown is any %-line we don't specifically recognize.
	EventUnknown EventKind = iota
	EventBegin
	EventEnd
	EventError
	EventWindowAdd
	EventWindowClose
	EventPaneDied
	EventSessionWindowChanged
	EventLayoutChange
	EventOutput
)

// Event is a parsed %-prefixed line from tmux control mode. Only fields
// relevant to Kind are populated.
type Event struct {
	Kind EventKind

	// %begin / %end / %error
	Time   string
	Number string
	Flags  string

	// %window-add / %window-close / %layout-change / %session-window-changed
	WindowID string

	// %pane-died / %output
	PaneID string

	// %session-window-changed
	SessionID string

	// %layout-change
	Layout string

	// %output
	Output string

	// Raw is the original line, useful for logging/debugging.
	Raw string
}

// IsReplyControl reports whether the event is one of %begin/%end/%error.
func (e Event) IsReplyControl() bool {
	switch e.Kind {
	case EventBegin, EventEnd, EventError:
		return true
	}
	return false
}

// ParseLine parses a single tmux control-mode line. Lines that don't begin
// with '%' return (Event{}, false) so the caller can treat them as reply
// payload data.
func ParseLine(line string) (Event, bool) {
	if !strings.HasPrefix(line, "%") {
		return Event{}, false
	}
	ev := Event{Raw: line}
	rest := line[1:]
	var head, tail string
	if i := strings.IndexByte(rest, ' '); i >= 0 {
		head, tail = rest[:i], rest[i+1:]
	} else {
		head = rest
	}

	switch head {
	case "begin", "end", "error":
		parts := strings.SplitN(tail, " ", 3)
		if len(parts) >= 1 {
			ev.Time = parts[0]
		}
		if len(parts) >= 2 {
			ev.Number = parts[1]
		}
		if len(parts) == 3 {
			ev.Flags = parts[2]
		}
		switch head {
		case "begin":
			ev.Kind = EventBegin
		case "end":
			ev.Kind = EventEnd
		case "error":
			ev.Kind = EventError
		}
	case "window-add":
		ev.Kind = EventWindowAdd
		ev.WindowID = firstField(tail)
	case "window-close":
		ev.Kind = EventWindowClose
		ev.WindowID = firstField(tail)
	case "pane-died":
		ev.Kind = EventPaneDied
		ev.PaneID = firstField(tail)
	case "session-window-changed":
		ev.Kind = EventSessionWindowChanged
		parts := strings.SplitN(tail, " ", 2)
		if len(parts) > 0 {
			ev.SessionID = parts[0]
		}
		if len(parts) > 1 {
			ev.WindowID = parts[1]
		}
	case "layout-change":
		ev.Kind = EventLayoutChange
		parts := strings.SplitN(tail, " ", 2)
		if len(parts) > 0 {
			ev.WindowID = parts[0]
		}
		if len(parts) > 1 {
			ev.Layout = parts[1]
		}
	case "output":
		ev.Kind = EventOutput
		parts := strings.SplitN(tail, " ", 2)
		ev.PaneID = parts[0]
		if len(parts) > 1 {
			ev.Output = parts[1]
		}
	default:
		ev.Kind = EventUnknown
	}
	return ev, true
}

func firstField(s string) string {
	if i := strings.IndexByte(s, ' '); i >= 0 {
		return s[:i]
	}
	return s
}
