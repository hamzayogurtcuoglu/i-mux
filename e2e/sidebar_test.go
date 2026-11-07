//go:build e2e

package e2e

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

const sidebarConfig = "[tmux]\nsidebar = true\nagent_bar = false\n"

// sidebarPanes starts claude with the sidebar and returns the agent pane and
// the sidebar pane.
func (h *H) startWithSidebar() (agentPane, bar string) {
	h.t.Helper()
	h.mustImux("claude", "-d")
	h.waitFor("sidebar pane", 10*time.Second, func() bool {
		agentPane, bar = "", ""
		for _, l := range strings.Split(strings.TrimSpace(h.mustTmux("list-panes", "-a", "-F", "#{pane_id} #{@imux_role} #{@imux_agent}")), "\n") {
			f := strings.Fields(l)
			switch {
			case len(f) >= 2 && f[1] == "sidebar":
				bar = f[0]
			case len(f) >= 2:
				agentPane = f[0]
			}
		}
		return agentPane != "" && bar != ""
	})
	return agentPane, bar
}

// rowOf returns the client screen position (1-based) of the first sidebar
// line containing text.
func (h *H) rowOf(bar, text string) (x, y int) {
	h.t.Helper()
	var lines []string
	h.waitFor("sidebar line "+text, 10*time.Second, func() bool {
		lines = strings.Split(h.capture(bar), "\n")
		for _, l := range lines {
			if strings.Contains(l, text) {
				return true
			}
		}
		return false
	})
	pos := strings.Fields(h.mustTmux("display-message", "-p", "-t", bar, "#{pane_left} #{pane_top}"))
	left, _ := strconv.Atoi(pos[0])
	top, _ := strconv.Atoi(pos[1])
	for i, l := range lines {
		if strings.Contains(l, text) {
			return left + 3, top + i + 1
		}
	}
	return 0, 0
}

// TestSidebar (T10.6): the sidebar shows the window's panes and the agents to
// continue in; a real click on "Antigravity" switches the agent pane with
// context; "Split right" asks for an agent and opens a pane with it; the
// sidebar never becomes a target; prefix b hides and shows it; closing the
// last agent pane closes the window.
func TestSidebar(t *testing.T) {
	h := newH(t, sidebarConfig)
	agentPane, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "CONTINUE HERE IN", 10*time.Second)
	if w := strings.TrimSpace(h.mustTmux("display-message", "-p", "-t", bar, "#{pane_width}")); w != "30" {
		t.Errorf("sidebar width %s, want 30", w)
	}
	if got := h.paneOpt(agentPane, "#{pane_active}"); got != "1" {
		t.Errorf("the agent pane should keep the focus, active=%s", got)
	}

	// A click on "Antigravity" under CONTINUE HERE IN switches the agent pane.
	x, y := h.rowOf(bar, "a Antigravity")
	c.Click(0, x, y)
	argv := h.run("antigravity", 1)
	if len(argv) != 2 || argv[0] != "-i" || !strings.HasPrefix(argv[1], "[i-mux handoff · Claude Code → Antigravity CLI") ||
		!strings.Contains(argv[1], "fake claude request 1") {
		t.Fatalf("antigravity argv %.300q", argv)
	}
	h.waitFor("agent pane runs antigravity", 5*time.Second, func() bool { return h.paneOpt(agentPane, "#{@imux_agent}") == "antigravity" })
	h.waitFor("sidebar shows antigravity as current", 5*time.Second, func() bool {
		for _, l := range strings.Split(h.capture(bar), "\n") {
			if strings.Contains(l, "Antigravity") && strings.Contains(l, "current") {
				return true
			}
		}
		return false
	})
	if h.paneOpt(bar, "#{@imux_agent}") != "" {
		t.Fatal("the sidebar must never run an agent")
	}

	// "Split right" asks which agent, then splits the agent pane. A person
	// does not click twice within tmux's double-click time.
	time.Sleep(500 * time.Millisecond)
	x, y = h.rowOf(bar, "Split right")
	mark := c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "Split right: new pane with", 5*time.Second)
	c.Write("x")
	h.run("codex", 1)
	h.rowOf(bar, "Codex")
	panes := strings.Split(strings.TrimSpace(h.mustTmux("list-panes", "-t", agentPane, "-F", "#{@imux_role}:#{@imux_agent}")), "\n")
	if strings.Join(panes, " ") != "sidebar: :antigravity :codex" {
		t.Fatalf("window panes %q", panes)
	}

	// prefix b hides the sidebar, and shows it again.
	c.Write(keyPrefix + "b")
	h.waitFor("sidebar hidden", 5*time.Second, func() bool {
		return !strings.Contains(h.mustTmux("list-panes", "-t", agentPane, "-F", "#{@imux_role}"), "sidebar")
	})
	c.Write(keyPrefix + "b")
	h.waitFor("sidebar shown", 5*time.Second, func() bool {
		return strings.Contains(h.mustTmux("list-panes", "-t", agentPane, "-F", "#{@imux_role}"), "sidebar")
	})

	// Closing every agent pane closes the window: the sidebar leaves with it.
	win := strings.TrimSpace(h.mustTmux("display-message", "-p", "-t", agentPane, "#{window_id}"))
	for _, l := range strings.Split(strings.TrimSpace(h.mustTmux("list-panes", "-t", agentPane, "-F", "#{pane_id} #{@imux_role}")), "\n") {
		if f := strings.Fields(l); len(f) == 1 {
			h.mustTmux("kill-pane", "-t", f[0])
		}
	}
	h.waitFor("window closed", 10*time.Second, func() bool {
		out, _ := h.tmux("list-windows", "-a", "-F", "#{window_id}")
		return !strings.Contains(out, win)
	})
}

// TestSidebarDoubleClick: people double-click buttons. A double click on
// "Split right" opens the menu once, the menu stays open (tmux's late
// DoubleClick1Pane must not close it) and the key typed next goes to the
// menu, not to the sidebar, where "x" would switch the agent pane instead.
func TestSidebarDoubleClick(t *testing.T) {
	h := newH(t, sidebarConfig)
	agentPane, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "CONTINUE HERE IN", 10*time.Second)
	x, y := h.rowOf(bar, "Split right")
	mark := c.Mark()
	c.Click(0, x, y)
	time.Sleep(80 * time.Millisecond)
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "Split right: new pane with", 5*time.Second)
	time.Sleep(600 * time.Millisecond) // past the double-click window
	c.Write("x")
	h.run("codex", 1)
	h.waitFor("codex in a new pane", 5*time.Second, func() bool {
		return strings.Contains(h.mustTmux("list-panes", "-t", agentPane, "-F", "#{@imux_agent}"), "codex")
	})
	if got := h.paneOpt(agentPane, "#{@imux_agent}"); got != "claude" {
		t.Fatalf("the key switched the agent pane to %q instead of splitting", got)
	}
	if n := strings.Count(h.mustTmux("list-panes", "-t", agentPane, "-F", "#{@imux_agent}"), "codex"); n != 1 || h.runs("codex") != 1 {
		t.Fatalf("split ran %d times (%d codex runs)", n, h.runs("codex"))
	}
}
