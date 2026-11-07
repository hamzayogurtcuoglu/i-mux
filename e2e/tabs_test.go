//go:build e2e

package e2e

import (
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

var styleRe = regexp.MustCompile(`#\[[^\]]*\]`)

// tabBar returns the tab bar of pane's session as drawn (styles and ranges
// removed): column i of the string is column i+1 of the client's top line.
func (h *H) tabBar(pane string) string {
	h.t.Helper()
	out := h.mustTmux("display-message", "-p", "-t", pane, "#{E:status-format[0]}")
	left, _, _ := strings.Cut(out, "#[align=right]")
	return styleRe.ReplaceAllString(strings.TrimSuffix(left, "\n"), "")
}

// tabCol returns the client column (1-based) of text in the tab bar.
func (h *H) tabCol(pane, text string) int {
	h.t.Helper()
	var bar string
	h.waitFor("tab bar shows "+text, 10*time.Second, func() bool {
		bar = h.tabBar(pane)
		return strings.Contains(bar, text)
	})
	return utf8.RuneCountInString(bar[:strings.Index(bar, text)]) + 1
}

// TestTabs (T14.4): the tab bar at the top names the tab after its agent; a
// real click on its + asks for an agent and opens a new tab with it and a
// sidebar; a click on the first tab goes back; prefix , names a tab.
func TestTabs(t *testing.T) {
	h := newH(t, sidebarConfig)
	agentPane, _ := h.startWithSidebar()
	h.run("claude", 1)
	c, client := h.attach(140, 40)
	c.waitScreen(h, "Agents", 10*time.Second)
	if pos := strings.TrimSpace(h.mustTmux("show-options", "-gv", "status-position")); pos != "top" {
		t.Fatalf("status-position %q", pos)
	}
	window := func() string {
		for _, l := range strings.Split(h.mustTmux("list-clients", "-F", "#{client_name} #{window_index}"), "\n") {
			if f := strings.Fields(l); len(f) == 2 && f[0] == client {
				return f[1]
			}
		}
		return ""
	}

	// + → "New tab with:" → x: Codex in a new tab, with its sidebar.
	x := h.tabCol(agentPane, " + ") + 1
	first := h.tabCol(agentPane, "0 Claude")
	time.Sleep(500 * time.Millisecond)
	mark := c.Mark()
	c.Click(0, x, 1)
	c.waitScreenAfter(h, mark, "New tab with:", 5*time.Second)
	time.Sleep(200 * time.Millisecond)
	c.Write("x")
	h.run("codex", 1)
	h.waitFor("client on the new tab", 5*time.Second, func() bool { return window() == "1" })
	panes := h.mustTmux("list-panes", "-t", ":1", "-F", "#{@imux_role}:#{@imux_agent}")
	if !strings.Contains(panes, "sidebar:") || !strings.Contains(panes, ":codex") {
		t.Fatalf("new tab panes %q", panes)
	}
	h.tabCol(agentPane, "1 Codex")

	// A click on the first tab goes back.
	time.Sleep(500 * time.Millisecond)
	c.Click(0, first+1, 1)
	h.waitFor("client back on the first tab", 5*time.Second, func() bool { return window() == "0" })

	// prefix , names the tab.
	mark = c.Mark()
	c.Write(keyPrefix + ",")
	c.waitScreenAfter(h, mark, "Tab name:", 5*time.Second)
	c.Write("\x15api work\r")
	h.tabCol(agentPane, "0 api work")
}
