//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// explorerH starts a project with files, the sidebar's explorer open, a
// fake editor, and an attached client.
func explorerH(t *testing.T) (*H, *Client, string) {
	t.Helper()
	ed := filepath.Join(t.TempDir(), "fake-editor")
	testutil.WriteFile(t, ed, []byte("#!/bin/sh\necho \"EDIT $*\"\nread x\n"))
	if err := os.Chmod(ed, 0o755); err != nil {
		t.Fatal(err)
	}
	h := newH(t, "editor = \""+ed+"\"\n"+sidebarConfig)
	testutil.WriteFile(t, filepath.Join(h.Homes.Project, "src", "app.go"), []byte("package app\n"))
	testutil.WriteFile(t, filepath.Join(h.Homes.Project, "README.md"), []byte("hello\n"))
	testutil.WriteFile(t, filepath.Join(h.Homes.Project, "my #notes.md"), []byte("notes\n"))
	_, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	h.openView(c, bar, "📁", "explorer")
	h.rowOf(bar, "README.md")
	time.Sleep(500 * time.Millisecond)
	return h, c, bar
}

// barLine is the sidebar's row holding text, with its escapes.
func (h *H) barLine(bar, text string) string {
	out, _ := h.tmux("capture-pane", "-e", "-p", "-t", bar)
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(stripANSI(l), text) {
			return l
		}
	}
	return ""
}

// TestExplorerHover (T34.3): the row under the pointer lights up and stays
// lit while the pointer rests; it goes when the pointer heads out of the
// sidebar; a clicked file keeps the selection background.
func TestExplorerHover(t *testing.T) {
	h, c, bar := explorerH(t)
	const hover, sel = "48;5;238", "48;5;24m"
	x, y := h.rowOf(bar, "README.md")
	c.Write(sgrMouse(35, x, y, 'M'))
	h.waitFor("README.md lit", 5*time.Second, func() bool { return strings.Contains(h.barLine(bar, "README.md"), hover) })
	if strings.Contains(h.barLine(bar, "app.go"), hover) || strings.Contains(h.barLine(bar, "my #notes.md"), hover) {
		t.Fatal("another row lit")
	}
	// At rest it stays lit.
	time.Sleep(time.Second)
	if !strings.Contains(h.barLine(bar, "README.md"), hover) {
		t.Fatal("the hover went while the pointer rested on the row")
	}
	// Down one row: the hover moves with it.
	_, y2 := h.rowOf(bar, "my #notes.md")
	c.Write(sgrMouse(35, x, y2, 'M'))
	h.waitFor("the next row lit", 5*time.Second, func() bool {
		return strings.Contains(h.barLine(bar, "my #notes.md"), hover) && !strings.Contains(h.barLine(bar, "README.md"), hover)
	})
	// On its way out to the right (the last reports near the edge).
	f := strings.Fields(h.mustTmux("display-message", "-p", "-t", bar, "#{pane_left} #{pane_width}"))
	left, _ := strconv.Atoi(f[0])
	w, _ := strconv.Atoi(f[1])
	c.Write(sgrMouse(35, left+w-8, y2, 'M'))
	time.Sleep(20 * time.Millisecond)
	c.Write(sgrMouse(35, left+w-3, y2, 'M'))
	h.waitFor("the hover gone", 5*time.Second, func() bool { return !strings.Contains(h.mustTmux("capture-pane", "-e", "-p", "-t", bar), hover) })

	// A click selects the file (and opens it); the selection stays.
	x, y = h.rowOf(bar, "README.md")
	mark := c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "EDIT README.md", 5*time.Second)
	c.Write("\r")
	h.waitFor("README.md selected", 5*time.Second, func() bool { return strings.Contains(h.barLine(bar, "README.md"), sel) })
}

// menuMouse reports whether this tmux's menus take clicks when i-mux opens
// them (display-menu -M, tmux 3.5).
func (h *H) menuMouse() bool {
	out, err := exec.Command(h.Tmux, "-V").Output()
	if err != nil {
		return false
	}
	v, ok := tmux.ParseVersion(string(out))
	return ok && v.AtLeast(tmux.MenuMinVersion[0], tmux.MenuMinVersion[1])
}

// menu right-clicks the sidebar row holding text and waits for its menu.
func (h *H) menu(c *Client, bar, text string, x, y int) int {
	h.t.Helper()
	if text != "" {
		x, y = h.rowOf(bar, text)
	}
	time.Sleep(400 * time.Millisecond) // past the double-click window
	mark := c.Mark()
	start := time.Now()
	c.Click(2, x, y)
	c.waitScreenAfter(h, mark, "Copy Path", 5*time.Second)
	h.t.Logf("menu on screen after %v", time.Since(start).Round(time.Millisecond))
	time.Sleep(200 * time.Millisecond)
	return mark
}

// TestExplorerMenus (T34.3): right-click menus like the editor's: copy the
// path and the relative path, rename, copy and paste, cut and paste, a new
// file in a new folder, delete into the Trash, collapse; names with "#" and
// spaces go through whole.
func TestExplorerMenus(t *testing.T) {
	h, c, bar := explorerH(t)
	p := h.Homes.Project
	copied := func() string { b, _ := os.ReadFile(filepath.Join(h.Log, "copied")); return string(b) }
	exists := func(rel string) bool { _, err := os.Stat(filepath.Join(p, rel)); return err == nil }

	// The menu opens at the pointer: its top border on the clicked row, its
	// first item (Open) right below; where tmux lets menus take clicks
	// (3.5), a click on it opens the file.
	x, y := h.rowOf(bar, "README.md")
	mark := h.menu(c, bar, "README.md", 0, 0)
	for _, item := range []string{"Open to the Side", "Open in Terminal", "Add to Agent", "Select for Compare", "Cut", "Copy Relative Path", "Rename…", "Delete"} {
		if !strings.Contains(stripANSI(c.Output()[mark:]), item) {
			t.Errorf("file menu lacks %q", item)
		}
	}
	if h.menuMouse() {
		// As a hand does: onto the item (tmux chooses the item the pointer
		// last moved to), then a click.
		mark = c.Mark()
		c.Write(sgrMouse(35, x+3, y+1, 'M'))
		time.Sleep(100 * time.Millisecond)
		c.Click(0, x+3, y+1)
		c.waitScreenAfter(h, mark, "EDIT README.md", 5*time.Second)
		c.Write("\r")
		h.menu(c, bar, "README.md", 0, 0)
	}
	c.Write("l") // Copy Relative Path
	h.waitFor("relative path copied", 5*time.Second, func() bool { return copied() == "README.md" })
	if b := strings.TrimSpace(h.mustTmux("show-buffer")); b != "README.md" {
		t.Errorf("tmux buffer %q", b)
	}

	// Copy Path of a name with "#" and a space: whole.
	h.menu(c, bar, "my #notes.md", 0, 0)
	c.Write("P")
	h.waitFor("path copied", 5*time.Second, func() bool { return copied() == filepath.Join(p, "my #notes.md") })

	// Rename: the prompt starts with the name.
	mark = h.menu(c, bar, "my #notes.md", 0, 0)
	c.Write("r")
	c.waitScreenAfter(h, mark, "New name", 5*time.Second)
	c.Write("\x15renamed #2.md\r") // Ctrl+U, the new name
	h.waitFor("renamed", 5*time.Second, func() bool { return exists("renamed #2.md") && !exists("my #notes.md") })
	h.rowOf(bar, "renamed #2.md")

	// Copy, then Paste on a folder: a copy in it; the folder opens.
	h.menu(c, bar, "renamed #2.md", 0, 0)
	c.Write("y")
	time.Sleep(300 * time.Millisecond)
	h.menu(c, bar, "src", 0, 0)
	c.Write("p")
	h.waitFor("pasted into src", 5*time.Second, func() bool { return exists("src/renamed #2.md") && exists("renamed #2.md") })
	h.rowOf(bar, "▾ 📂 src")
	// Paste again: a copy with a free name.
	h.menu(c, bar, "▾ 📂 src", 0, 0)
	c.Write("p")
	h.waitFor("a second copy", 5*time.Second, func() bool { return exists("src/renamed #2 copy.md") })

	// New File in a new folder from the empty space; it opens in the editor.
	x, last := h.rowOf(bar, "README.md")
	mark = h.menu(c, bar, "", x, last+4)
	if !strings.Contains(stripANSI(c.Output()[mark:]), "Collapse Folders") {
		t.Fatal("no empty-space menu")
	}
	c.Write("n")
	c.waitScreenAfter(h, mark, "New file name", 5*time.Second)
	mark = c.Mark()
	c.Write("sub/new.txt\r")
	c.waitScreenAfter(h, mark, "EDIT sub/new.txt", 5*time.Second)
	c.Write("\r")
	if !exists("sub/new.txt") {
		t.Fatal("no new file")
	}
	h.rowOf(bar, "▾ 📂 sub")
	h.waitFor("new.txt selected", 5*time.Second, func() bool { return strings.Contains(h.barLine(bar, "new.txt"), "48;5;24m") })

	// Cut, then Paste into another folder: moved.
	h.menu(c, bar, "app.go", 0, 0)
	c.Write("x")
	time.Sleep(300 * time.Millisecond)
	h.menu(c, bar, "▾ 📂 sub", 0, 0)
	c.Write("p")
	h.waitFor("moved", 5*time.Second, func() bool { return exists("sub/app.go") && !exists("src/app.go") })

	// Delete: asked, then into the Trash.
	mark = h.menu(c, bar, "README.md", 0, 0)
	c.Write("d")
	c.waitScreenAfter(h, mark, "Delete 'README.md'?", 5*time.Second)
	time.Sleep(200 * time.Millisecond)
	c.Write("y")
	h.waitFor("in the Trash", 5*time.Second, func() bool {
		_, err := os.Stat(filepath.Join(h.Log, "trash", "README.md"))
		return err == nil && !exists("README.md")
	})

	// Collapse Folders closes them all.
	x, y = h.rowOf(bar, "▾ 📂 sub")
	h.menu(c, bar, "", x, y+8)
	c.Write("z")
	h.waitFor("folders closed", 5*time.Second, func() bool {
		s := h.screen(bar)
		return strings.Contains(s, "▸ 📁 src") && strings.Contains(s, "▸ 📁 sub")
	})
}
