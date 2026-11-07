//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestExplorerAndSearch (T21.5, T22.5): the rail's 📁 shows the project's
// files; a click opens a folder and another opens a file in the editor; 🔍
// opens the search popup, which finds a word as it is typed, folds a file's
// results and opens the file in the editor at its line.
func TestExplorerAndSearch(t *testing.T) {
	// A fake editor shows how it was called.
	ed := filepath.Join(t.TempDir(), "fake-editor")
	testutil.WriteFile(t, ed, []byte("#!/bin/sh\necho \"EDIT $*\"\nread x\n"))
	if err := os.Chmod(ed, 0o755); err != nil {
		t.Fatal(err)
	}
	h := newH(t, "editor = \""+ed+"\"\n"+sidebarConfig)
	testutil.WriteFile(t, filepath.Join(h.Homes.Project, "src", "app.go"), []byte("package app\n\n// the needle is here\nfunc Run() {}\n"))
	testutil.WriteFile(t, filepath.Join(h.Homes.Project, "README.md"), []byte("hello\n"))
	_, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)

	x, y := h.rowOf(bar, "📁")
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
	c.waitScreenAfter(h, mark, "EDIT src/app.go", 5*time.Second) // a click edits
	c.Write("\r")
	time.Sleep(500 * time.Millisecond)

	x, y = h.rowOf(bar, "🔍")
	mark = c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "Search in proj", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	c.Write("needle")
	c.waitScreenAfter(h, mark, "1 results in 1 files", 5*time.Second)
	c.waitScreenAfter(h, mark, "src/app.go", 5*time.Second)
	// Tab folds the results, Tab again unfolds them.
	mark = c.Mark()
	c.Write("\t")
	c.waitScreenAfter(h, mark, "▸ src/app.go", 5*time.Second)
	mark = c.Mark()
	c.Write("\t")
	c.waitScreenAfter(h, mark, "▾ src/app.go", 5*time.Second)
	mark = c.Mark()
	c.Write("\r")
	c.waitScreenAfter(h, mark, "EDIT +3 src/app.go", 5*time.Second) // at the line
	c.Write("\r")
}

// TestViewerWheel (T22.5): a right-click on a file offers Edit and View;
// in the viewer the mouse wheel scrolls.
func TestViewerWheel(t *testing.T) {
	h := newH(t, sidebarConfig)
	var b strings.Builder
	for i := 1; i <= 200; i++ {
		b.WriteString("line " + strconv.Itoa(i) + "\n")
	}
	testutil.WriteFile(t, filepath.Join(h.Homes.Project, "long.txt"), []byte(b.String()))
	_, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	x, y := h.rowOf(bar, "📁")
	c.Click(0, x, y)
	time.Sleep(500 * time.Millisecond)
	x, y = h.rowOf(bar, "long.txt")
	mark := c.Mark()
	c.Click(2, x, y)
	c.waitScreenAfter(h, mark, "View", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	mark = c.Mark()
	c.Write("v")
	c.waitScreenAfter(h, mark, "line 20", 5*time.Second)
	mark = c.Mark()
	for i := 0; i < 30; i++ {
		c.Write(sgrMouse(65, 70, 20, 'M'))
		time.Sleep(10 * time.Millisecond)
	}
	c.waitScreenAfter(h, mark, "line 60", 5*time.Second)
	c.Write("q")
}
