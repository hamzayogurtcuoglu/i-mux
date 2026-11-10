package watch

import (
	"strings"
	"testing"
)

func TestTyping(t *testing.T) {
	claude := func(progress, input, hint string) string {
		return strings.Join([]string{"> earlier question", "⏺ An answer.", "", progress, "╭────────────╮", "│ > " + input + " │", "╰────────────╯", "  " + hint, "", ""}, "\n")
	}
	copilot := func(progress, input string) string {
		return strings.Join([]string{"● An answer.", progress, "────", "❯ " + input, "────", " ← open sidebar · / commands"}, "\n")
	}
	codex := func(progress, input string) string {
		return strings.Join([]string{"• An answer.", progress, "", "› " + input, "  ⏎ send   ⌃C quit"}, "\n")
	}
	for _, c := range []struct {
		name     string
		old, cur string
		typing   bool
	}{
		{"claude: typing", claude("", "fix", "? for shortcuts"), claude("", "fix the", "? for shortcuts"), true},
		{"claude: typing changes the hint", claude("", "", "? for shortcuts"), claude("", "f", "esc to clear"), true},
		{"claude: thinking", claude("✻ Thinking… (3s)", "", ""), claude("✶ Thinking… (4s)", "", ""), false},
		{"claude: thinking while typing", claude("✻ Thinking… (3s)", "a", ""), claude("✶ Thinking… (4s)", "ab", ""), false},
		{"copilot: typing", copilot("", "hel"), copilot("", "hello"), true},
		{"copilot: working", copilot("◐ Working", ""), copilot("◓ Working", ""), false},
		{"codex: typing", codex("", "x"), codex("", "xy"), true},
		{"codex: working", codex("• Working (5s)", ""), codex("• Working (6s)", ""), false},
		{"no input line", "working 1\n", "working 1\nworking 2\n", false},
		{"the input moved", "a\n> x\n", "a\nb\n> x\n", false},
	} {
		if got := Typing(c.old, c.cur); got != c.typing {
			t.Errorf("%s: Typing = %v", c.name, got)
		}
	}
}
