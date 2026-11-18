//go:build e2e

package e2e

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

const (
	keyCtrlBackslash = "\x1c"
	keyPrefix        = "\x02" // C-b
	uiCols, uiRows   = 120, 40
)

// TestUIKeyPath (T2.8d): Ctrl+\ then a switches the Claude pane to Antigravity,
// which receives the handoff. No command is typed.
func TestUIKeyPath(t *testing.T) {
	h := newH(t, "")
	pane := h.startSession("claude")
	h.run("claude", 1)
	c, _ := h.attach(uiCols, uiRows)
	c.waitScreen(h, "fake claude ready", 5*time.Second)
	mark := c.openWith(h, keyCtrlBackslash, "Switch to Antigravity (with context)")
	c.Write("a")
	argv := h.run("antigravity", 1)
	if len(argv) != 2 || !strings.HasPrefix(argv[1], "[i-mux handoff · Claude Code → Antigravity CLI") ||
		!strings.Contains(argv[1], "fake claude request 1") {
		t.Fatalf("antigravity argv %.300q", argv)
	}
	h.waitFor("agent bar to show ▶ Antigravity", 5*time.Second, func() bool { return h.paneOpt(pane, "#{@imux_agent}") == "antigravity" })
	c.waitScreenAfter(h, mark, "▶ Antigravity", 5*time.Second)
}

// TestUIRightClickBorder (T2.8d, Q28): a right-click on a pane border opens
// the menu of the pane that owns that border (run-shell -t = in the
// MouseDown3Border binding), not of the active pane. In tmux a "Border" is a
// pane's right or bottom edge; the vertical border between two side-by-side
// panes belongs to the left one. (tmux 3.2a ignores clicks on the
// pane-border-status line itself.)
func TestUIRightClickBorder(t *testing.T) {
	h := newH(t, "")
	pane := h.startSession("claude")
	h.run("claude", 1)
	right := strings.TrimSpace(h.mustImux("split", "antigravity", "-h", "--pane", pane))
	h.run("antigravity", 1)
	h.mustTmux("select-pane", "-t", right) // the Antigravity pane is active
	c, _ := h.attach(uiCols, uiRows)
	c.waitScreen(h, "fake antigravity ready", 5*time.Second)
	time.Sleep(400 * time.Millisecond) // outside tmux's double-click window
	f := strings.Fields(h.paneOpt(right, "#{pane_left} #{pane_top}"))
	left, top := atoi(t, f[0]), atoi(t, f[1])
	mark := c.Mark()
	// 1-based column `left` is the 0-based column left-1: the border.
	c.Click(2, left, top+5)
	c.waitScreenAfter(h, mark, "i-mux · Claude", 5*time.Second)
	if strings.Contains(stripANSI(c.Output()[mark:]), "i-mux · Antigravity") {
		t.Fatal("menu opened for the active pane instead of the clicked border's pane")
	}
	c.Write("q")
}

// TestUIPickerToCodex (T2.8d): prefix S → first row → "Continue in Codex"
// launches fake Codex with a handoff built from the picked conversation.
func TestUIPickerToCodex(t *testing.T) {
	h := newH(t, "")
	pane := h.startSession("claude")
	h.run("claude", 1)
	// The newest conversation of the project is a synthetic Antigravity one.
	h.synthetic("antigravity", 7, "picked antigravity conversation about caching", time.Now().Add(time.Minute))
	c, _ := h.attach(uiCols, uiRows)
	c.waitScreen(h, "fake claude ready", 5*time.Second)
	c.openWith(h, keyPrefix+"S", "picked antigravity conversation")
	mark := c.Mark()
	c.Write("1")
	c.waitScreenAfter(h, mark, "Continue in Codex (with context)", 5*time.Second)
	c.Write("x")
	argv := h.run("codex", 1)
	if len(argv) != 1 || !strings.HasPrefix(argv[0], "[i-mux handoff · Antigravity CLI → Codex CLI") ||
		!strings.Contains(argv[0], "picked antigravity conversation about caching") {
		t.Fatalf("codex argv %.300q", argv)
	}
	h.waitFor("pane runs codex", 5*time.Second, func() bool { return h.paneOpt(pane, "#{@imux_agent}") == "codex" })
}

// TestUIAgentBarClick (T2.8d): a left click on "Antigravity" in the agent bar
// switches the active pane. Needs tmux >= 3.4 (user ranges). Its range is
// "imux:agy": "imux:antigravity" (16 bytes) exceeded tmux's 15-byte limit,
// so tmux dropped the range and the click did nothing.
func TestUIAgentBarClick(t *testing.T) {
	h := newH(t, "")
	if v := tmuxVersion(t, h); !v.AtLeast(tmux.ClickMinVersion[0], tmux.ClickMinVersion[1]) {
		t.Skipf("tmux %s has no clickable status ranges (needs %d.%d)", v, tmux.ClickMinVersion[0], tmux.ClickMinVersion[1])
	}
	pane := h.startSession("claude")
	h.run("claude", 1)
	c, client := h.attach(uiCols, uiRows)
	c.waitScreen(h, "▶ Claude", 5*time.Second)
	x := barColumn(t, h, client, pane, "Antigravity")
	mark := c.Mark()
	c.Click(0, x, uiRows) // the agent bar is the last line
	argv := h.run("antigravity", 1)
	if len(argv) != 2 || !strings.Contains(argv[1], "fake claude request 1") {
		t.Fatalf("antigravity argv %.300q", argv)
	}
	c.waitScreenAfter(h, mark, "▶ Antigravity", 5*time.Second)
}

// barColumn returns the 1-based column of label on the rendered agent bar.
func barColumn(t *testing.T, h *H, client, pane, label string) int {
	t.Helper()
	out := h.mustTmux("display-message", "-p", "-c", client, "-t", pane, "#{E:status-format[1]}")
	plain := regexp.MustCompile(`#\[[^\]]*\]`).ReplaceAllString(strings.TrimRight(out, "\n"), "")
	i := strings.Index(plain, " "+label+" ")
	if i < 0 {
		t.Fatalf("%q not on the bar: %q", label, plain)
	}
	return utf8.RuneCountInString(plain[:i]) + 2 // the space before the label, 1-based
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
