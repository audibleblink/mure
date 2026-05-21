package harnesses

import (
	"fmt"
	"strings"
)

// TaskArgKind identifies how a task string is passed to a harness command.
type TaskArgKind int

const (
	TaskArgNone TaskArgKind = iota
	TaskArgPositional
	TaskArgStdin
	TaskArgFlag
)

// TaskArg is a parsed task_arg manifest value.
type TaskArg struct {
	Kind TaskArgKind
	Flag string // populated when Kind==TaskArgFlag
}

// ParseTaskArg parses a task_arg manifest string.
// Accepted forms: "", "none", "positional", "stdin", "flag:<name>".
func ParseTaskArg(s string) (TaskArg, error) {
	switch s {
	case "", "none":
		return TaskArg{Kind: TaskArgNone}, nil
	case "positional":
		return TaskArg{Kind: TaskArgPositional}, nil
	case "stdin":
		return TaskArg{Kind: TaskArgStdin}, nil
	}
	if rest, ok := strings.CutPrefix(s, "flag:"); ok {
		if rest == "" {
			return TaskArg{}, fmt.Errorf("invalid task_arg %q: flag name required", s)
		}
		return TaskArg{Kind: TaskArgFlag, Flag: rest}, nil
	}
	return TaskArg{}, fmt.Errorf("invalid task_arg %q", s)
}
