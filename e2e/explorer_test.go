//go:build e2e

package e2e

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestExplorerAndSearch (T21.5): the rail's ▤ shows the project's files; a
// click opens a folder and another shows a file with line numbers; ⌕ opens
// the search popup, which finds a word as it is typed and opens the file at
// its line.
func TestExplorerAndSearch(t *testing.T) {
	h := newH(t, sidebarConfig)
	testutil.WriteFile(t, filepath.Join(h.Homes.Project, "src", "app.go"), []byte("package app\n\n// the needle is here\nfunc Run() {}\n"))
	testutil.WriteFile(t, filepath.Join(h.Homes.Project, "README.md"), []byte("hello\n"))
	_, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)

	x, y := h.rowOf(bar, "▤")
	c.Click(0, x, y)
	h.rowOf(bar, "EXPLORER · proj")
	if strings.Contains(h.screen(bar), "app.go") {
		t.Fatal("src starts closed")
	}
	time.Sleep(500 * time.Millisecond)
	x, y = h.rowOf(bar, "▸ src")
	c.Click(0, x, y)
	h.rowOf(bar, "▾ src")
	time.Sleep(500 * time.Millisecond)
	x, y = h.rowOf(bar, "app.go")
	mark := c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "3 // the needle is here", 5*time.Second)
	c.Write("q")
	time.Sleep(500 * time.Millisecond)

	x, y = h.rowOf(bar, "⌕")
	mark = c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "Search in proj", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	c.Write("needle")
	c.waitScreenAfter(h, mark, "1 results in 1 files", 5*time.Second)
	c.waitScreenAfter(h, mark, "src/app.go", 5*time.Second)
	mark = c.Mark()
	c.Write("\r")
	c.waitScreenAfter(h, mark, "func Run() {}", 5*time.Second)
	c.Write("q")
}
