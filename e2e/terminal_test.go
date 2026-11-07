//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"
)

// TestTerminalFromMenus (T18.1): the Agents + and the tab bar's + both end
// with "Terminal (shell)": t opens a plain shell, as a split (listed as
// Terminal in the sidebar) and as a new tab with its own sidebar.
func TestTerminalFromMenus(t *testing.T) {
	h := newH(t, sidebarConfig)
	agentPane, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, client := h.attach(140, 40)
	c.waitScreen(h, "Agents", 10*time.Second)
	shells := func(target string) int {
		n := 0
		for _, l := range strings.Split(h.mustTmux("list-panes", "-t", target, "-F", "#{@imux_role}:#{@imux_agent}:#{pane_dead}"), "\n") {
			if l == "::0" {
				n++
			}
		}
		return n
	}

	x, y := h.plusOf(bar, "Agents")
	mark := c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "Terminal (shell)", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	c.Write("t")
	h.waitFor("a shell next to claude", 5*time.Second, func() bool { return shells(agentPane) == 1 })
	h.rowOf(bar, "›  Terminal")

	window := func() string {
		for _, l := range strings.Split(h.mustTmux("list-clients", "-F", "#{client_name} #{window_index}"), "\n") {
			if f := strings.Fields(l); len(f) == 2 && f[0] == client {
				return f[1]
			}
		}
		return ""
	}
	x = h.tabCol(agentPane, " + ") + 1
	time.Sleep(500 * time.Millisecond)
	mark = c.Mark()
	c.Click(0, x, 1)
	c.waitScreenAfter(h, mark, "Terminal (shell)", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	c.Write("t")
	h.waitFor("a shell tab with a sidebar", 5*time.Second, func() bool {
		if window() != "1" {
			return false
		}
		rows := strings.Fields(strings.ReplaceAll(h.mustTmux("list-panes", "-t", ":1", "-F", "[#{@imux_role}:#{@imux_agent}]"), "\n", " "))
		return strings.Join(rows, " ") == "[sidebar:] [:]"
	})
}
