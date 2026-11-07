//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// clientSession returns the session the client shows.
func (h *H) clientSession(client string) string {
	for _, l := range strings.Split(h.mustTmux("list-clients", "-F", "#{client_name} #{client_session}"), "\n") {
		if f := strings.Fields(l); len(f) == 2 && f[0] == client {
			return f[1]
		}
	}
	return ""
}

// TestOpenPickerAndClose (T15.4): the Projects + opens the folder picker;
// typing part of a git folder's name and Enter opens it as a project; a
// click on the current project opens its menu, and Close (confirmed) ends it
// and brings the client back to the first project.
func TestOpenPickerAndClose(t *testing.T) {
	h := newH(t, sidebarConfig)
	for _, d := range []string{"code/acme-api/.git", "code/blog/.git", "Library/hidden/.git"} {
		if err := os.MkdirAll(filepath.Join(h.Homes.Home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	_, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, client := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	first := h.clientSession(client)

	x, y := h.plusOf(bar, "PROJECTS")
	mark := c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "Open a project", 5*time.Second)
	c.waitScreenAfter(h, mark, "~/code/acme-api", 10*time.Second) // found by the scan
	time.Sleep(300 * time.Millisecond)
	c.Write("blo")
	c.waitScreenAfter(h, mark, "▸ ~/code/blog", 5*time.Second)
	c.Write("\r")
	h.run("claude", 2)
	h.waitFor("client in the blog project", 5*time.Second, func() bool { return strings.HasPrefix(h.clientSession(client), "blog-") })
	blog := h.clientSession(client)
	if strings.Contains(stripANSI(c.Output()[mark:]), "~/Library/hidden") {
		t.Error("Library is not scanned")
	}

	// The blog project's sidebar: its row opens the menu; Close asks first.
	var newBar string
	h.waitFor("blog sidebar", 5*time.Second, func() bool {
		for _, l := range strings.Split(h.mustTmux("list-panes", "-s", "-t", blog, "-F", "#{pane_id} #{@imux_role}"), "\n") {
			if f := strings.Fields(l); len(f) == 2 && f[1] == "sidebar" {
				newBar = f[0]
			}
		}
		return newBar != ""
	})
	time.Sleep(500 * time.Millisecond)
	x, y = h.rowOf(newBar, " blog ")
	mark = c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "Close project", 5*time.Second)
	time.Sleep(600 * time.Millisecond)
	c.Write("x")
	c.waitScreenAfter(h, mark, "Close blog?", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	c.Write("y")
	h.waitFor("blog closed, client back", 5*time.Second, func() bool {
		out, _ := h.tmux("has-session", "-t", "="+blog)
		_ = out
		_, err := h.tmux("has-session", "-t", "="+blog)
		return err != nil && h.clientSession(client) == first
	})
}

// TestCloseTab (T15.4): the × of the current tab asks, then closes it; the
// client shows the remaining tab.
func TestCloseTab(t *testing.T) {
	h := newH(t, sidebarConfig)
	agentPane, _ := h.startWithSidebar()
	h.run("claude", 1)
	c, client := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	window := func() string {
		for _, l := range strings.Split(h.mustTmux("list-clients", "-F", "#{client_name} #{window_index}"), "\n") {
			if f := strings.Fields(l); len(f) == 2 && f[0] == client {
				return f[1]
			}
		}
		return ""
	}
	h.mustImux("tab", "codex", "--pane", agentPane)
	h.run("codex", 1)
	h.waitFor("client on the new tab", 5*time.Second, func() bool { return window() == "1" })

	h.tabCol(agentPane, "1 Codex") // named by its sidebar
	x := h.tabCol(agentPane, "×")
	time.Sleep(500 * time.Millisecond)
	mark := c.Mark()
	c.Click(0, x, 1)
	c.waitScreenAfter(h, mark, "Close this tab?", 5*time.Second)
	time.Sleep(600 * time.Millisecond)
	c.Write("y")
	h.waitFor("tab closed", 5*time.Second, func() bool {
		return strings.TrimSpace(h.mustTmux("display-message", "-p", "-t", agentPane, "#{session_windows}")) == "1" && window() == "0"
	})
}
