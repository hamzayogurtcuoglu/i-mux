package watch

import (
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// TestBusy (D55, D76): each agent's busy line, only near the bottom (a
// transcript that quotes it further up does not count); an agent without one
// says nothing. Claude Code 2.1's progress line has the time it has worked
// instead of "esc to interrupt".
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
		{"antigravity", blank + "Thinking...\nPress esc to interrupt generation.\n> \n", true, true},
		{"antigravity", blank + "> \nPress ? to see keyboard shortcuts\n", false, true},
		{"copilot", blank + "────\n❯ \n────\n Autopilot · ◉ Working · 67.0 KiB ctrl+c interrupt      Claude\n", true, true},
		{"copilot", blank + "it shows ctrl+c interrupt while it works\n\n────\n❯ \n────\n Autopilot · 67.0 KiB\n", false, true},
		{"opencode", blank + "esc interrupt\n", false, false},
	} {
		busy, says := Busy(c.agent, c.screen)
		if busy != c.busy || says != c.says {
			t.Errorf("%s %q: busy %v, says %v", c.agent, c.screen[len(blank):], busy, says)
		}
	}
}
