//go:build e2e

package e2e

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestGoToFile (T35.2, T68.4): Ctrl+P in an agent's pane opens Go to File
// in the project; a few letters and ":line" find the file, Enter opens it in
// the editor at that line; the explorer's ⋯ → Go to File… opens it too.
func TestGoToFile(t *testing.T) {
	h, c, bar := explorerH(t)
	testutil.WriteFile(t, filepath.Join(h.Homes.Project, "docs", "guide #1.md"), []byte("x\n"))

	// Ctrl+P in the agent's pane (the sidebar is not focused).
	agent := strings.TrimSpace(h.mustTmux("display-message", "-p", "-t", "{last}", "#{pane_id}"))
	for _, p := range strings.Fields(h.mustTmux("list-panes", "-F", "#{pane_id}:#{@imux_agent}")) {
		if strings.HasSuffix(p, ":claude") {
			agent = strings.TrimSuffix(p, ":claude")
		}
	}
	h.mustTmux("select-pane", "-t", agent)
	mark := c.Mark()
	c.Write("\x10")
	c.waitScreenAfter(h, mark, "Go to File · proj", 5*time.Second)
	c.waitScreenAfter(h, mark, "changed last first", 5*time.Second)
	time.Sleep(200 * time.Millisecond)
	mark = c.Mark()
	c.Write("apgo:3")
	c.waitScreenAfter(h, mark, "1 files", 5*time.Second)
	mark = c.Mark()
	c.Write("\r")
	c.waitScreenAfter(h, mark, "EDIT +3 src/app.go", 5*time.Second)
	c.Write("\r")
	time.Sleep(500 * time.Millisecond)

	// The explorer's ⋯ → Go to File…, and a name with "#" and a space.
	x, y := h.buttonOf(bar, "⋯", "⋯")
	mark = c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "Go to File…", 5*time.Second)
	time.Sleep(600 * time.Millisecond) // past the popup's first 400 ms
	mark = c.Mark()
	c.Write("p")
	c.waitScreenAfter(h, mark, "Go to File · proj", 5*time.Second)
	time.Sleep(200 * time.Millisecond)
	mark = c.Mark()
	c.Write("guide")
	c.waitScreenAfter(h, mark, "1 files", 5*time.Second)
	mark = c.Mark()
	c.Write("\r")
	c.waitScreenAfter(h, mark, "EDIT docs/guide #1.md", 5*time.Second)
	c.Write("\r")

	// Esc closes it without opening anything.
	h.mustTmux("select-pane", "-t", agent)
	time.Sleep(300 * time.Millisecond)
	mark = c.Mark()
	c.Write("\x10")
	c.waitScreenAfter(h, mark, "Go to File · proj", 5*time.Second)
	time.Sleep(200 * time.Millisecond)
	c.Write("\x1b")
	time.Sleep(800 * time.Millisecond)
	if strings.Contains(stripANSI(c.Output()[mark:]), "EDIT ") {
		t.Fatal("Esc opened a file")
	}
}
