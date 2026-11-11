//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
)

// TestTabsOverTheTerminal (T58.2): the first tab starts above the agent's
// pane, past the sidebar; it follows the panel when it closes to the rail,
// and starts at the left edge when a pane is zoomed.
func TestTabsOverTheTerminal(t *testing.T) {
	h := newH(t, sidebarConfig)
	agentPane, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	num := func(p, f string) int {
		n, _ := strconv.Atoi(strings.TrimSpace(h.mustTmux("display-message", "-p", "-t", p, f)))
		return n
	}
	// " 0 Claude": the tab's first column is a blank, then its number.
	over := func(what string, want func() int) {
		t.Helper()
		h.waitFor(what, 5*time.Second, func() bool { return h.tabCol(agentPane, "0 Claude") == want() })
	}
	over("the tab over the agent", func() int { return num(agentPane, "#{pane_left}") + 3 })
	if num(agentPane, "#{pane_left}") < 20 {
		t.Fatalf("the agent pane starts at %d", num(agentPane, "#{pane_left}"))
	}

	x, y := h.rowOf(bar, "🤖")
	c.Click(0, x, y)
	h.waitFor("panel closed", 5*time.Second, func() bool { return num(bar, "#{pane_width}") == sidebar.ClosedWidth })
	over("the tab over the agent, the panel closed", func() int { return num(agentPane, "#{pane_left}") + 3 })

	h.mustTmux("resize-pane", "-Z", "-t", agentPane)
	over("the tab at the left edge, zoomed", func() int { return 3 })
}

// TestMenuClick (T60.1): before tmux 3.5 a menu is i-mux's popup (D54), and
// a click on its item chooses it (tmux's own menu closed without choosing).
func TestMenuClick(t *testing.T) {
	h := newH(t, sidebarConfig)
	if tmuxVersion(t, h).AtLeast(3, 5) {
		t.Skip("tmux's own menus take the mouse")
	}
	agentPane, _ := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	time.Sleep(500 * time.Millisecond)
	mark := c.Mark()
	c.Click(0, 140-4, 1) // ⏻ Exit
	c.waitScreenAfter(h, mark, "Back to the terminal", 5*time.Second)
	time.Sleep(600 * time.Millisecond) // past the popup's first 400 ms
	// The popup: its border, the title row, then Back, a separator and Quit
	// (6 rows). tmux centers it with its top at (rows-1)/2 + h/2 - h.
	const rows, h0 = 40, 6
	y := (rows-1)/2 + h0/2 - h0 + 1 + 2
	mark = c.Mark()
	c.Click(0, 70, y)
	c.waitScreenAfter(h, mark, "detached", 5*time.Second)
	if got := h.paneOpt(agentPane, "#{@imux_agent}|#{pane_dead}"); got != "claude|0" {
		t.Fatalf("the agent after detaching: %q", got)
	}
}

// TestExitButton (T56.1, T56.2): ⏻ Exit at the right of the tab bar opens
// its menu; Back to the terminal detaches the client and the agents keep
// running. Quit ends the server; `imux` started elsewhere then offers the
// last session, and Enter brings the project back with its agent.
func TestExitButton(t *testing.T) {
	h := newH(t, sidebarConfig)
	agentPane, _ := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	time.Sleep(500 * time.Millisecond)

	// " ⏻ Exit " then a blank end the bar: the x of Exit is 4 from the end.
	mark := c.Mark()
	c.Click(0, 140-4, 1)
	c.waitScreenAfter(h, mark, "Leave i-mux", 5*time.Second)
	c.waitScreenAfter(h, mark, "Back to the terminal", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	mark = c.Mark()
	c.Write("b")
	c.waitScreenAfter(h, mark, "detached", 5*time.Second)
	if got := h.paneOpt(agentPane, "#{@imux_agent}|#{pane_dead}"); got != "claude|0" {
		t.Fatalf("the agent after detaching: %q", got)
	}
	if out, _ := h.tmux("list-clients", "-F", "#{client_name}"); strings.TrimSpace(out) != "" {
		t.Fatalf("clients left: %q", out)
	}

	// Quit: the project is recorded and the server ends.
	h.mustImux("quit", "--yes")
	h.waitFor("the server gone", 5*time.Second, func() bool {
		_, err := h.tmux("list-sessions")
		return err != nil
	})
	elsewhere := filepath.Join(h.Homes.Home, "elsewhere")
	if err := os.MkdirAll(elsewhere, 0o700); err != nil {
		t.Fatal(err)
	}
	c2 := h.startClient(120, 40, elsewhere, h.Bin)
	c2.waitScreen(h, "Restore last session", 10*time.Second)
	c2.waitScreen(h, "this folder", 5*time.Second)
	c2.Write("\r")
	h.run("claude", 2)
	h.waitFor("the project back", 10*time.Second, func() bool {
		out, _ := h.tmux("list-sessions", "-F", "#{session_path}")
		return strings.Contains(out, h.Homes.Project)
	})
}

// TestStartPicker (T56.2): `imux` in a new folder while a project is open
// asks which project to open, this folder first; Enter opens it.
func TestStartPicker(t *testing.T) {
	h := newH(t, sidebarConfig)
	h.startWithSidebar()
	h.run("claude", 1)
	dir := filepath.Join(h.Homes.Home, "newproj")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	c := h.startClient(120, 40, dir, h.Bin)
	c.waitScreen(h, "Open a project", 10*time.Second)
	c.waitScreen(h, "this folder", 5*time.Second)
	if strings.Contains(c.Text(), "Restore last session") {
		t.Error("restore offered while the projects are open")
	}
	c.Write("\r")
	h.waitFor("a project for the folder", 10*time.Second, func() bool {
		out, _ := h.tmux("list-sessions", "-F", "#{session_path}")
		return strings.Contains(out, dir)
	})
	h.run("claude", 2)
}
