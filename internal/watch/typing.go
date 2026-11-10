package watch

import "strings"

// promptMarks start the input line of the agents' prompts, after the box
// edges and spaces before them: ">" (Claude Code, Gemini), "❯" (Copilot),
// "›" and "▌" (Codex).
var promptMarks = []string{">", "❯", "›", "▌"}

// promptScan is how many non-blank lines from the bottom may hold the input
// line: below it come hints, a footer and lists of completions.
const promptScan = 12

// promptLine is the index of the agent's input line: the last line, among
// the last promptScan non-blank ones, that starts like a prompt; -1 when
// there is none.
func promptLine(lines []string) int {
	seen := 0
	for i := len(lines) - 1; i >= 0 && seen < promptScan; i-- {
		t := strings.TrimLeft(lines[i], " \t│┃║")
		if strings.TrimSpace(t) == "" {
			continue
		}
		seen++
		for _, m := range promptMarks {
			if strings.HasPrefix(t, m) {
				return i
			}
		}
	}
	return -1
}

// Typing reports whether a screen change touched only the agent's input line
// and what is below it (hints, a list of completions): the user typing. An
// agent at work changes its progress line, which is above the input, or its
// transcript.
func Typing(old, cur string) bool {
	a, b := strings.Split(old, "\n"), strings.Split(cur, "\n")
	if len(a) != len(b) {
		return false
	}
	p := promptLine(b)
	if p < 0 || promptLine(a) != p {
		return false
	}
	for i := 0; i < p; i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
