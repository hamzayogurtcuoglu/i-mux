package watch

import (
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// TestBusy (D55): each agent's busy line, only near the bottom (a transcript
// that quotes it further up does not count); an agent without one says nothing.
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
