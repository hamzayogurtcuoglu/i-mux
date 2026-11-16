package watch

import (
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// TestBusy (D55, D76, D78): each agent's busy line, as its CLI shows it now
// and before, only near the bottom (a transcript that quotes it further up
// does not count); an agent i-mux does not know says nothing. Claude Code
// 2.1's progress line has the time it has worked instead of "esc to
// interrupt"; Antigravity's status line says "esc to cancel", and a question
// on its screen wins; Codex's interrupt key can be another; OpenCode's footer
// says "esc interrupt".
func TestBusy(t *testing.T) {
	blank := strings.Repeat("transcript\n", 30)
	for _, c := range []struct {
		agent      session.AgentID
		screen     string
		busy, says bool
	}{
		{"claude", blank + "✻ Thinking… (12s · ↑ 1.2k tokens · esc to interrupt)\n  ⎿ ☐ fix the tests\n\n───\n> \n───\n  ? for shortcuts\n", true, true},
		{"claude", blank + "● Done.\n\n───\n> \n───\n  ? for shortcuts\n", false, true},
		{"claude", "esc to interrupt)\n" + blank + "> \n", false, true},
		{"claude", blank + "· Discombobulating… (30m 46s · ↓ 139.3k tokens · thinking with high effort)\n  ⎿  Tip: Use /clear to start fresh\n" +
			"              ✓ Update installed · Restart to update\n───\n❯ \n───\n  5h 25% · week 77%\n  ⏵⏵ auto mode on (shift+tab to cycle)\n", true, true},
		{"claude", blank + "✢ Compacting conversation… (1h 2m 5s)\n───\n❯ \n───\n", true, true},
		{"claude", blank + "✻ Worked for 30m 46s\n\n● Done… (see the diff)\n───\n❯ \n───\n  ⏵⏵ auto mode on (shift+tab to cycle)\n", false, true},
		{"codex", blank + "• Working (5s • esc to interrupt)\n\n▌ \n", true, true},
		{"codex", blank + "• Running hooks (1m 05s • f12 to interrupt) · 1 background terminal running\n\n› \n", true, true},
		{"codex", blank + "• Ran cargo test\n\n› Ask Codex to do anything\n  ⏎ send   ⌃J newline   ⌃T transcript\n", false, true},
		{"antigravity", blank + "Thinking...\nPress esc to interrupt generation.\n> \n", true, true},
		{"antigravity", blank + "> \nPress ? to see keyboard shortcuts\n", false, true},
		{"antigravity", blank + "⣯  Editing files...\n└ Tip: Use /diff to view uncommitted changes\n────\n>\n────\nesc to cancel                    \n", true, true},
		{"antigravity", blank + "────\n>\n────\n● [10:52:35] make -C /home/user/project test\n────\nesc to cancel\n", true, true},
		{"antigravity", blank + "────\n>\n────\nEsc disabled during file edits\n", true, true},
		{"antigravity", blank + "⣯  Reading file… (3s · esc to cancel)\n────\n>\n────\n", true, true},
		{"antigravity", blank + "● Done.\n────\n>\n────\n? for shortcuts\n", false, true},
		{"antigravity", blank + "Do you want to proceed?\n❯ Yes, accept this change\n  No\n────\nesc to cancel\n", false, true},
		{"antigravity", blank + "/model\n  gemini-3-pro\n  gemini-3-flash\nesc to dismiss\n", false, true},
		{"copilot", blank + "────\n❯ \n────\n Autopilot · ◉ Working · 67.0 KiB ctrl+c interrupt      Claude\n", true, true},
		{"copilot", blank + "it shows ctrl+c interrupt while it works\n\n────\n❯ \n────\n Autopilot · 67.0 KiB\n", false, true},
		{"opencode", blank + "┃ fix the tests\n▀▀▀▀▀▀▀▀\n ⣾ esc interrupt                  Build  Claude Sonnet\n", true, true},
		{"opencode", blank + "┃\n▀▀▀▀▀▀▀▀\n [⋯] esc again to interrupt\n", true, true},
		{"opencode", blank + "┃\n▀▀▀▀▀▀▀▀\nanthropic claude-sonnet                          ctrl+p commands\n", false, true},
		{"gemini", blank + "esc to cancel\n", false, false},
	} {
		busy, says := Busy(c.agent, c.screen)
		if busy != c.busy || says != c.says {
			t.Errorf("%s %q: busy %v, says %v", c.agent, c.screen[len(blank):], busy, says)
		}
	}
}
