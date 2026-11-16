package watch

import (
	"regexp"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// busyLine is what an agent CLI shows while it works, and only then, and
// among how many of the screen's last non-blank lines. With asks, a
// question at the bottom of the screen (Asking) means the agent waits for
// the user even while that shows.
type busyLine struct {
	re    *regexp.Regexp
	lines int
	asks  bool
}

// claudeBusy is Claude Code's progress line: "esc to interrupt" on older
// versions; on 2.1 only its word, "…" and the time it has worked in
// parentheses ("· Discombobulating… (30m 46s · ↓ 139.3k tokens)", D76).
var claudeBusy = regexp.MustCompile(`(?i)\besc to interrupt\b|…\s*\(\d+[hms](?:\s+\d+[hms])*(?:\s*·|\))`)

// codexBusy is Codex's status line, "• Working (5s • esc to interrupt)",
// whichever key interrupts it ("f12 to interrupt", D78).
var codexBusy = regexp.MustCompile(`(?i)\bto interrupt\b`)

// antigravityBusy is Antigravity's status line while it works ("esc to
// cancel", or "Esc disabled during file edits"; "? for shortcuts" when
// idle), a progress line ending "(12s · esc to cancel)", or its older "Press
// esc to interrupt generation." (D78).
var antigravityBusy = regexp.MustCompile(`(?i)\besc to interrupt\b|^\s*esc (?:to cancel|disabled during)\b|…\s*\((?:[^()]*·\s*)?esc to cancel\)`)

// openCodeBusy is OpenCode's prompt footer while a turn runs: "esc
// interrupt", "esc again to interrupt" once pressed ("ctrl+p commands" when
// idle; a permission request takes the prompt's place), D78.
var openCodeBusy = regexp.MustCompile(`(?i)\besc (?:again to )?interrupt\b`)

// busyLines are the agents whose screen says when they work (D55, D78), as
// their CLIs show it now: Claude Code's progress line (its to-do list can
// stand between it and the input), Codex's status line, Antigravity's
// status line (it may keep its hint while it asks, so a question wins),
// Copilot's footer ("◉ Working · … ctrl+c interrupt", its last line),
// OpenCode's prompt footer. An agent without one here (none of today's) is
// at work while its screen changes (D38).
var busyLines = map[session.AgentID]busyLine{
	"claude":      {re: claudeBusy, lines: 16},
	"codex":       {re: codexBusy, lines: 10},
	"antigravity": {re: antigravityBusy, lines: 6, asks: true},
	"copilot":     {re: regexp.MustCompile(`(?i)\bctrl\+c (?:to )?interrupt\b`), lines: 3},
	"opencode":    {re: openCodeBusy, lines: 5},
}

// Busy reports whether a screen shows its agent at work; says is false for an
// agent whose busy line i-mux does not know.
func Busy(agent session.AgentID, screen string) (busy, says bool) {
	b, ok := busyLines[agent]
	if !ok {
		return false, false
	}
	if b.asks && Asking(screen) {
		return false, true
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
