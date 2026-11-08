//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"
)

// TestRenameHighPaneID: a pane whose id starts with %1 (%10-%19, %1xx) is
// renamed from its row and with the sidebar's n. The command prompt turns
// "%1" in its template into the answer, so "%14" became "<name>4" and tmux
// said "no such pane".
func TestRenameHighPaneID(t *testing.T) {
	h := newH(t, sidebarConfig)
	agentPane, bar := h.startWithSidebar()
	h.run("claude", 1)
	for i := 0; i < 10; i++ { // use up pane ids
		id := strings.TrimSpace(h.mustTmux("split-window", "-d", "-t", agentPane, "-P", "-F", "#{pane_id}", "sleep 60"))
		h.mustTmux("kill-pane", "-t", id)
	}
	np := strings.TrimSpace(h.mustImux("split", "codex", "-h", "--pane", agentPane))
	h.run("codex", 1)
	if !strings.HasPrefix(np, "%1") || len(np) < 3 {
		t.Fatalf("pane id %s", np)
	}
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)

	// From its row's menu.
	x, y := h.rowOf(bar, "◎  Codex")
	mark := c.Mark()
	c.Click(2, x, y)
	c.waitScreenAfter(h, mark, "Rename…", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	c.Write("r")
	c.waitScreenAfter(h, mark, "Name for this pane:", 5*time.Second)
	c.Write("\x15my codex\r")
	h.waitFor("named from the row", 5*time.Second, func() bool { return h.paneOpt(np, "#{@imux_name}") == "my codex" })
	if v := strings.TrimSpace(h.mustTmux("show-options", "-gqv", "@imux_rename_pane")); v != "" {
		t.Fatalf("@imux_rename_pane left: %q", v)
	}
	h.rowOf(bar, "◎  my codex")

	// With n in the sidebar: it names the pane the sidebar follows.
	h.mustTmux("select-pane", "-t", np)
	time.Sleep(300 * time.Millisecond)
	h.mustTmux("select-pane", "-t", bar)
	time.Sleep(300 * time.Millisecond)
	mark = c.Mark()
	c.Write("n")
	c.waitScreenAfter(h, mark, "Name for this pane:", 5*time.Second)
	c.Write("\x15codex two\r")
	h.waitFor("named with n", 5*time.Second, func() bool { return h.paneOpt(np, "#{@imux_name}") == "codex two" })
}
