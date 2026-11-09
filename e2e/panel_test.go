//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
)

// closedRowOf is rowOf for a sidebar whose panel is closed (the rail alone).
func (h *H) closedRowOf(bar, text string) (x, y int) {
	h.t.Helper()
	var lines []string
	h.waitFor("closed rail shows "+text, 10*time.Second, func() bool {
		lines = strings.Split(h.screen(bar), "\n")
		for _, l := range lines {
			if strings.Contains(l, text) {
				return true
			}
		}
		return false
	})
	pos := strings.Fields(h.mustTmux("display-message", "-p", "-t", bar, "#{pane_left} #{pane_top} #{status-position} #{status}"))
	left, _ := strconv.Atoi(pos[0])
	top, _ := strconv.Atoi(pos[1])
	if pos[2] == "top" && pos[3] != "off" {
		top++
	}
	for i, l := range lines {
		if j := strings.Index(l, text); j >= 0 {
			return left + sidebar.DisplayWidth(l[:j]) + 1, top + i + 1
		}
	}
	return 0, 0
}

// TestClosePanel (T24.2): a real click on the active view's icon closes the
// panel: the sidebar narrows to the rail and the agent pane gets the room; a
// click on another view's icon opens it again with that view.
func TestClosePanel(t *testing.T) {
	h := newH(t, sidebarConfig)
	agentPane, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	width := func(p string) int {
		n, _ := strconv.Atoi(strings.TrimSpace(h.mustTmux("display-message", "-p", "-t", p, "#{pane_width}")))
		return n
	}
	x, y := h.rowOf(bar, "🤖")
	open, agentW := width(bar), width(agentPane)

	c.Click(0, x, y)
	h.waitFor("panel closed", 5*time.Second, func() bool { return width(bar) == sidebar.ClosedWidth })
	h.waitFor("agent pane grown", 5*time.Second, func() bool { return width(agentPane) == agentW+open-sidebar.ClosedWidth })
	if v := h.paneOpt(bar, "#{@imux_collapsed}"); v != "1" {
		t.Fatalf("@imux_collapsed %q", v)
	}
	h.waitFor("only the rail drawn", 5*time.Second, func() bool {
		s := h.screen(bar)
		return !strings.Contains(s, "AGENTS") && strings.Contains(s, "📁")
	})
	// A redraw keeps it closed (the sidebar does not take its width back).
	h.mustImux("__sidebar-refresh")
	time.Sleep(500 * time.Millisecond)
	if w := width(bar); w != sidebar.ClosedWidth {
		t.Fatalf("width after a redraw %d", w)
	}

	x, y = h.closedRowOf(bar, "📁")
	c.Click(0, x, y)
	h.waitFor("panel open with the explorer", 5*time.Second, func() bool {
		return width(bar) == open && h.paneOpt(bar, "#{@imux_view}") == sidebar.ViewExplorer
	})
	h.waitFor("agent pane back", 5*time.Second, func() bool { return width(agentPane) == agentW })
	if v := h.paneOpt(bar, "#{@imux_collapsed}"); v != "" {
		t.Fatalf("@imux_collapsed %q after opening", v)
	}
	// Another view's icon while open only switches the view.
	time.Sleep(500 * time.Millisecond)
	x, y = h.rowOf(bar, "🤖")
	c.Click(0, x, y)
	h.waitFor("agents view", 5*time.Second, func() bool { return h.paneOpt(bar, "#{@imux_view}") == sidebar.ViewAgents })
	time.Sleep(time.Second)
	if w := width(bar); w != open {
		t.Fatalf("switching the view changed the width to %d", w)
	}
}

// openView clicks a rail icon until the sidebar shows that view: a click
// right after a client attached can land while the sidebar takes its width
// back, and is lost.
func (h *H) openView(c *Client, bar, icon, view string) {
	h.t.Helper()
	for attempt := 0; attempt < 3; attempt++ {
		x, y := h.rowOf(bar, icon)
		c.Click(0, x, y)
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if h.paneOpt(bar, "#{@imux_view}") == view {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		time.Sleep(500 * time.Millisecond) // past the double-click window
	}
	h.t.Fatalf("the %s view did not open", view)
}

// TestDragPanelEdge (T36.2): dragging the edge between the panel and the
// agent with the mouse widens the panel and it stays that wide: after the
// drag, after a redraw, in the config, in the other tab and in a new tab.
func TestDragPanelEdge(t *testing.T) {
	h := newH(t, sidebarConfig)
	agentPane, bar := h.startWithSidebar()
	h.run("claude", 1)
	h.mustImux("tab", "codex", "--pane", agentPane)
	h.run("codex", 1)
	var bar2 string
	h.waitFor("the second tab's sidebar", 10*time.Second, func() bool {
		for _, l := range strings.Split(h.mustTmux("list-panes", "-a", "-F", "#{pane_id} #{@imux_role}"), "\n") {
			if f := strings.Fields(l); len(f) == 2 && f[1] == "sidebar" && f[0] != bar {
				bar2 = f[0]
			}
		}
		return bar2 != ""
	})
	h.mustTmux("select-window", "-t", agentPane)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	h.rowOf(bar, "🤖") // drawn at its width
	width := func(p string) int {
		n, _ := strconv.Atoi(strings.TrimSpace(h.mustTmux("display-message", "-p", "-t", p, "#{pane_width}")))
		return n
	}
	if w := width(bar); w != 43 {
		t.Fatalf("width before the drag %d", w)
	}

	// Press on the edge (the border column right of the panel), move 17
	// columns right with the button down, release.
	pos := strings.Fields(h.mustTmux("display-message", "-p", "-t", bar, "#{pane_left} #{pane_top} #{status-position}"))
	left, _ := strconv.Atoi(pos[0])
	top, _ := strconv.Atoi(pos[1])
	if pos[2] == "top" {
		top++
	}
	x, y := left+43+1, top+10
	time.Sleep(500 * time.Millisecond)
	c.Write(sgrMouse(0, x, y, 'M'))
	for i := 1; i <= 17; i++ {
		time.Sleep(15 * time.Millisecond)
		c.Write(sgrMouse(32, x+i, y, 'M'))
	}
	time.Sleep(15 * time.Millisecond)
	c.Write(sgrMouse(0, x+17, y, 'm'))
	h.waitFor("panel dragged to 60", 5*time.Second, func() bool { return width(bar) == 60 })
	h.waitFor("panel drawn at 60", 5*time.Second, func() bool {
		first, _, _ := strings.Cut(h.screen(bar), "\n")
		return sidebar.DisplayWidth(strings.TrimRight(first, " ")) == 59 // the "+" of Projects
	})

	// Saved once the drag rests; the other tab's panel takes the width.
	cfgPath := filepath.Join(h.Homes.Imux, "config.toml")
	h.waitFor("width saved in the config", 5*time.Second, func() bool {
		b, _ := os.ReadFile(cfgPath)
		return regexp.MustCompile(`(?m)^sidebar_width\s*=\s*60$`).Match(b)
	})
	h.waitFor("the other tab's panel at 60", 5*time.Second, func() bool { return width(bar2) == 60 })

	// A redraw keeps it.
	h.mustImux("__sidebar-refresh")
	time.Sleep(700 * time.Millisecond)
	if w := width(bar); w != 60 {
		t.Fatalf("width after a redraw %d", w)
	}

	// A new tab's panel opens at it.
	h.mustImux("tab", "copilot", "--pane", agentPane)
	h.run("copilot", 1)
	h.waitFor("a new tab's panel at 60", 10*time.Second, func() bool {
		n := 0
		for _, l := range strings.Split(h.mustTmux("list-panes", "-a", "-F", "#{@imux_role} #{pane_width}"), "\n") {
			if f := strings.Fields(l); len(f) == 2 && f[0] == "sidebar" && f[1] == "60" {
				n++
			}
		}
		return n == 3
	})
}
