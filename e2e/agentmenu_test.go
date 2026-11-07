//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"
)

// TestAgentMenu (T17.2): a right-click on an agent row opens its menu;
// Rename names the pane (shown in the sidebar); Close (confirmed) closes
// that agent and keeps the other.
func TestAgentMenu(t *testing.T) {
	h := newH(t, sidebarConfig)
	agentPane, bar := h.startWithSidebar()
	h.run("claude", 1)
	h.mustImux("split", "codex", "-h", "--pane", agentPane)
	h.run("codex", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)

	// Rename Claude from its row.
	x, y := h.rowOf(bar, "✻  Claude")
	mark := c.Mark()
	c.Click(2, x, y)
	c.waitScreenAfter(h, mark, "Rename…", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	c.Write("r")
	c.waitScreenAfter(h, mark, "Name for this pane:", 5*time.Second)
	c.Write("\x15api work\r")
	h.waitFor("pane named", 5*time.Second, func() bool { return h.paneOpt(agentPane, "#{@imux_name}") == "api work" })
	h.rowOf(bar, "✻  api work")

	// Close Codex from its row.
	var codexPane string
	for _, l := range strings.Split(h.mustTmux("list-panes", "-t", agentPane, "-F", "#{pane_id} #{@imux_agent}"), "\n") {
		if f := strings.Fields(l); len(f) == 2 && f[1] == "codex" {
			codexPane = f[0]
		}
	}
	time.Sleep(500 * time.Millisecond)
	x, y = h.rowOf(bar, "◎  Codex")
	mark = c.Mark()
	c.Click(2, x, y)
	c.waitScreenAfter(h, mark, "Close agent…", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	c.Write("x")
	c.waitScreenAfter(h, mark, "Its conversation stays in History", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	c.Write("y")
	h.waitFor("codex closed, claude kept", 5*time.Second, func() bool {
		out := h.mustTmux("list-panes", "-t", agentPane, "-F", "#{pane_id}")
		return !strings.Contains(out, codexPane+"\n") && strings.Contains(out, agentPane)
	})
}
