//go:build e2e

package e2e

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestPaneKeys (M72, D67): prefix L / H swap the active pane with the one
// on that side, the focus staying on it, and never with the sidebar; prefix
// r resizes with l / L until Esc; prefix ? opens the keys, typing filters
// them and Enter runs the one chosen.
func TestPaneKeys(t *testing.T) {
	h := newH(t, sidebarConfig)
	claude, bar := h.startWithSidebar()
	h.run("claude", 1)
	h.mustTmux("split-window", "-h", "-d", "-t", claude, "sleep 600")
	other := ""
	order := func() string {
		return strings.Join(strings.Fields(h.mustTmux("list-panes", "-t", claude, "-F", "#{pane_id}")), " ")
	}
	for _, p := range strings.Fields(order()) {
		if p != claude && p != bar {
			other = p
		}
	}
	c, client := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	h.mustTmux("select-pane", "-t", claude)
	if got := order(); got != bar+" "+claude+" "+other {
		t.Fatalf("panes %q", got)
	}
	// prefix L: Claude goes right of the other pane, and keeps the focus.
	c.Write(keyPrefix + "L")
	h.waitFor("claude swapped right", 5*time.Second, func() bool { return order() == bar+" "+other+" "+claude })
	if a := strings.TrimSpace(h.mustTmux("display-message", "-p", "-c", client, "#{pane_id}")); a != claude {
		t.Fatalf("focus on %s, want the moved pane %s", a, claude)
	}
	// prefix L at the window's edge: nothing (tmux's {right-of} would go
	// around); prefix H twice: back, then not past the sidebar.
	c.Write(keyPrefix + "L")
	c.Write(keyPrefix + "H")
	h.waitFor("claude back left", 5*time.Second, func() bool { return order() == bar+" "+claude+" "+other })
	c.Write(keyPrefix + "H")
	time.Sleep(700 * time.Millisecond)
	if got := order(); got != bar+" "+claude+" "+other {
		t.Fatalf("swapped with the sidebar: %q", got)
	}

	// prefix r: l grows Claude by 2, L by 10; Esc leaves the mode, and an l
	// then goes to Claude, not to the size.
	width := func() int {
		n, _ := strconv.Atoi(strings.TrimSpace(h.mustTmux("display-message", "-p", "-t", claude, "#{pane_width}")))
		return n
	}
	table := func() string {
		return strings.TrimSpace(h.mustTmux("display-message", "-p", "-c", client, "#{client_key_table}"))
	}
	w0 := width()
	mark := c.Mark()
	c.Write(keyPrefix + "r")
	c.waitScreenAfter(h, mark, "Resize: h j k l", 5*time.Second)
	if tb := table(); tb != "imux-resize" {
		t.Fatalf("key table %q", tb)
	}
	c.Write("l")
	h.waitFor("2 wider", 5*time.Second, func() bool { return width() == w0+2 })
	c.Write("L")
	h.waitFor("10 more", 5*time.Second, func() bool { return width() == w0+12 })
	c.Write("\x1b")
	h.waitFor("out of the resize mode", 5*time.Second, func() bool { return table() == "root" })
	c.Write("l")
	time.Sleep(500 * time.Millisecond)
	if got := width(); got != w0+12 {
		t.Fatalf("width %d after Esc, want %d", got, w0+12)
	}

	// prefix ?: the keys; typing filters them, Enter runs the row.
	mark = c.Mark()
	c.Write(keyPrefix + "?")
	c.waitScreenAfter(h, mark, "Keys · prefix = Ctrl+B", 5*time.Second)
	c.waitScreenAfter(h, mark, "Swap with the pane on the left", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	mark = c.Mark()
	c.Write("split right")
	time.Sleep(400 * time.Millisecond)
	c.Write("\r")
	c.waitScreenAfter(h, mark, "Split right: new pane with", 5*time.Second)
}
