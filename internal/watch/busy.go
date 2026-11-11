package watch

import (
	"regexp"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// busyLine is what an agent CLI shows while it works, and only then, and
// among how many of the screen's last non-blank lines.
type busyLine struct {
	re    *regexp.Regexp
	lines int
}

var escToInterrupt = regexp.MustCompile(`(?i)\besc to interrupt\b`)

// busyLines are the agents whose screen says when they work (D55): Claude
// Code's and Codex's progress line ("… (12s · esc to interrupt)"; Claude's
// to-do list can stand between it and the input), Antigravity's hint ("Press
// esc to interrupt generation."), Copilot's footer ("◉ Working · … ctrl+c
// interrupt", its last line). The others (OpenCode) are at work while their
// screen changes (D38).
var busyLines = map[session.AgentID]busyLine{
	"claude":      {escToInterrupt, 16},
	"codex":       {escToInterrupt, 10},
	"antigravity": {escToInterrupt, 6},
	"copilot":     {regexp.MustCompile(`(?i)\bctrl\+c (?:to )?interrupt\b`), 3},
}

// Busy reports whether a screen shows its agent at work; says is false for an
// agent whose busy line i-mux does not know.
func Busy(agent session.AgentID, screen string) (busy, says bool) {
	b, ok := busyLines[agent]
	if !ok {
		return false, false
	}
	n := 0
	lines := strings.Split(strings.TrimRight(screen, " \t\r\n"), "\n")
	for i := len(lines) - 1; i >= 0 && n < b.lines; i-- {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		n++
		if b.re.MatchString(lines[i]) {
			return true, true
		}
	}
	return false, true
}
