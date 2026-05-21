package harnesses

import (
	"fmt"
	"regexp"
	"strings"
)

// markerBegin returns the begin marker line for harness h.
func markerBegin(h string) string { return fmt.Sprintf("# >>> mure:%s >>>", h) }
func markerEnd(h string) string   { return fmt.Sprintf("# <<< mure:%s <<<", h) }

// WrapBlock wraps body in begin/end markers for harness h.
// The returned block always ends with a newline.
func WrapBlock(h, body string) string {
	body = strings.TrimRight(body, "\n")
	return markerBegin(h) + "\n" + body + "\n" + markerEnd(h) + "\n"
}

func blockRegex(h string) *regexp.Regexp {
	q := regexp.QuoteMeta(h)
	return regexp.MustCompile(`(?ms)^# >>> mure:` + q + ` >>>\n.*?^# <<< mure:` + q + ` <<<\n?`)
}

// ReplaceOrAppendBlock returns existing with any prior block for h replaced
// by a new wrapped block; if no prior block exists, the wrapped block is
// appended (with a separating newline if needed).
func ReplaceOrAppendBlock(existing, h, body string) string {
	block := WrapBlock(h, body)
	re := blockRegex(h)
	if re.MatchString(existing) {
		return re.ReplaceAllString(existing, block)
	}
	if existing == "" {
		return block
	}
	if !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	return existing + block
}

// StripBlock removes the block for h from existing (if present).
func StripBlock(existing, h string) string {
	return blockRegex(h).ReplaceAllString(existing, "")
}
