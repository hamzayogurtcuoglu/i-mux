//go:build e2e

package e2e

import (
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// sidebars counts the sidebar panes per window.
func (h *H) sidebars() map[string]int {
	n := map[string]int{}
	for _, l := range strings.Split(strings.TrimSpace(h.mustTmux("list-panes", "-a", "-F", "#{window_id} #{@imux_role}")), "\n") {
		if f := strings.Fields(l); len(f) == 2 && f[1] == "sidebar" {
			n[f[0]]++
		}
	}
	return n
}

// TestSidebarComesBack (T41.1): a sidebar killed the way macOS kills a
// program whose file was overwritten (SIGKILL) comes back; one turned off
// stays off; a new tab never gets two.
func TestSidebarComesBack(t *testing.T) {
	t.Setenv("IMUX_SIDEBAR_CHECK", "500ms")
	h := newH(t, sidebarConfig)
	agent, bar := h.startWithSidebar()
	h.run("claude", 1)
	win := strings.TrimSpace(h.mustTmux("display-message", "-p", "-t", agent, "#{window_id}"))
	h.mustImux("__watch", "ensure")

	pid, _ := strconv.Atoi(h.paneOpt(bar, "#{@imux_sidebar_pid}"))
	if pid <= 1 {
		t.Fatalf("sidebar pid %d", pid)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	h.waitFor("the killed sidebar's pane gone", 5*time.Second, func() bool { return h.sidebars()[win] == 0 })
	h.waitFor("the sidebar back", 15*time.Second, func() bool { return h.sidebars()[win] == 1 })

	// Turned off: it stays off.
	h.mustImux("sidebar", "off", "--pane", agent)
	h.waitFor("sidebar off", 5*time.Second, func() bool { return h.sidebars()[win] == 0 })
	time.Sleep(3 * time.Second)
	if n := h.sidebars()[win]; n != 0 {
		t.Fatalf("a sidebar turned off came back (%d)", n)
	}
	h.mustImux("sidebar", "on", "--pane", agent)
	h.waitFor("sidebar on", 5*time.Second, func() bool { return h.sidebars()[win] == 1 })

	// New tabs: one sidebar each, never two.
	for i := 0; i < 3; i++ {
		h.mustImux("tab", "shell", "--pane", agent)
	}
	time.Sleep(4 * time.Second)
	for w, n := range h.sidebars() {
		if n != 1 {
			t.Fatalf("window %s has %d sidebars", w, n)
		}
	}
	if n := len(h.sidebars()); n != 4 {
		t.Fatalf("%d windows with a sidebar, want 4", n)
	}
}
