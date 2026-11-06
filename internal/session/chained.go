package session

import (
	"strings"
	"time"
)

// MarkerPrefix starts every i-mux handoff document.
const MarkerPrefix = "[i-mux handoff ·"

// MarkerTimeLayout is the time format inside the marker.
const MarkerTimeLayout = "2006-01-02 15:04"

// Marker renders the first line of a handoff document.
func Marker(from, to AgentID, at time.Time) string {
	return MarkerPrefix + " " + DisplayName(from) + " → " + DisplayName(to) + " · " + at.Format(MarkerTimeLayout) + "]"
}

// IsHandoff reports whether text is an imux handoff document.
func IsHandoff(text string) bool {
	return strings.HasPrefix(strings.TrimLeft(text, " \t\r\n\uFEFF"), MarkerPrefix)
}

// ChainedHandoff is an earlier handoff found as the first user message of a
// session: the conversation started from another agent's context.
type ChainedHandoff struct {
	From, To        AgentID
	At              time.Time
	OriginalRequest string
	History         string // the earlier doc's "Earlier history" + "Conversation", as text
}

// ParseChained parses a handoff document. It returns nil if text is not one.
func ParseChained(text string) *ChainedHandoff {
	text = strings.TrimLeft(text, " \t\r\n\uFEFF")
	if !strings.HasPrefix(text, MarkerPrefix) {
		return nil
	}
	c := &ChainedHandoff{}
	first, rest, _ := strings.Cut(text, "\n")
	inner := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(first, MarkerPrefix)), "]")
	parts := strings.Split(inner, " · ")
	if len(parts) >= 1 {
		if from, to, ok := strings.Cut(parts[0], " → "); ok {
			c.From = agentByDisplayName(strings.TrimSpace(from))
			c.To = agentByDisplayName(strings.TrimSpace(to))
		}
	}
	if len(parts) >= 2 {
		if t, err := time.ParseInLocation(MarkerTimeLayout, strings.TrimSpace(parts[1]), time.Local); err == nil {
			c.At = t
		}
	}
	sections := splitSections(rest)
	var hist []string
	for _, s := range sections {
		switch {
		case strings.HasPrefix(s.heading, "Original request"):
			c.OriginalRequest = strings.TrimSpace(s.body)
		case strings.HasPrefix(s.heading, "Earlier history"):
			if b := strings.TrimSpace(s.body); b != "" {
				hist = append(hist, b)
			}
		case strings.HasPrefix(s.heading, "Conversation"), strings.HasPrefix(s.heading, "While you were away"):
			if b := strings.TrimSpace(s.body); b != "" {
				hist = append(hist, b)
			}
		}
	}
	c.History = strings.Join(hist, "\n\n")
	return c
}

type section struct{ heading, body string }

// splitSections splits markdown by "## " headings.
func splitSections(s string) []section {
	var out []section
	var cur *section
	var b strings.Builder
	flush := func() {
		if cur != nil {
			cur.body = b.String()
			out = append(out, *cur)
		}
		b.Reset()
	}
	for _, line := range strings.SplitAfter(s, "\n") {
		trimmed := strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(trimmed, "## ") {
			flush()
			cur = &section{heading: strings.TrimSpace(trimmed[3:])}
			continue
		}
		if cur != nil {
			b.WriteString(line)
		}
	}
	flush()
	return out
}

func agentByDisplayName(n string) AgentID {
	for _, a := range AllAgents {
		if DisplayName(a) == n || ShortName(a) == n || string(a) == n {
			return a
		}
	}
	return AgentID(strings.ToLower(n))
}
