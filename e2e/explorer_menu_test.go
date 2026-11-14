//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/theme"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// explorerH starts a project with files (and the extra ones, paths in the
// project), the sidebar's explorer open, a fake editor, and an attached
// client.
func explorerH(t *testing.T, extra ...string) (*H, *Client, string) {
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
	for _, p := range extra {
		testutil.WriteFile(t, filepath.Join(h.Homes.Project, p), []byte("x\n"))
	}
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
	hover, sel := hoverAndSelection()
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
	h.waitFor("new.txt selected", 5*time.Second, func() bool {
		_, sel := hoverAndSelection()
		return strings.Contains(h.barLine(bar, "new.txt"), sel)
	})

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

// TestExplorerTitle (T68.4): the explorer's title has VS Code's buttons and
// ⋯, and says what the button under the pointer does. Dot files show only
// after ⋯ → Show Hidden Files (a click on it), .git never; config.toml keeps
// the choice and a new tab's explorer follows it; choosing it again hides
// them. New Folder… asks for a name, Refresh shows a file written
// elsewhere at once, Collapse Folders closes the open folders.
func TestExplorerTitle(t *testing.T) {
	h, c, bar := explorerH(t, ".github/workflows/ci.yml", ".env", ".git/HEAD")
	p := h.Homes.Project
	cfg := filepath.Join(h.Homes.Imux, "config.toml")
	saved := func(v string) bool {
		return regexp.MustCompile(`(?m)^show_hidden\s*=\s*` + v + `$`).MatchString(h.readFile(cfg))
	}
	gitRow := func(s string) bool {
		for _, l := range strings.Split(s, "\n") {
			if strings.HasSuffix(strings.TrimRight(l, " "), " .git") {
				return true
			}
		}
		return false
	}
	// The title's line ends with ⋯; a button's place on it.
	button := func(glyph string) (int, int) { return h.buttonOf(bar, "⋯", glyph) }
	if l := h.lineOf(bar, "⋯"); !strings.Contains(l, "EXPLORER · proj") || !strings.Contains(l, "📝  📁  🔄  ⊟  ⋯") {
		t.Fatalf("title %q", l)
	}
	if s := h.screen(bar); strings.Contains(s, ".env") || strings.Contains(s, ".github") || gitRow(s) {
		t.Fatalf("hidden files shown at first:\n%s", s)
	}

	// The pointer on New Folder…: its label in the title, then the title
	// again once the pointer is on the tree.
	x, y := button("📁")
	c.Write(sgrMouse(35, x+1, y, 'M'))
	h.waitFor("New Folder… in the title", 5*time.Second, func() bool { return strings.Contains(h.lineOf(bar, "⋯"), "New Folder…") })
	tx, ty := h.rowOf(bar, "README.md")
	c.Write(sgrMouse(35, tx+2, ty, 'M'))
	h.waitFor("the title back", 5*time.Second, func() bool { return strings.Contains(h.lineOf(bar, "⋯"), "EXPLORER · proj") })

	// ⋯, then a click on Show Hidden Files (Go to File…, a line, then it):
	// tmux's menu has its first item right below the clicked row, i-mux's
	// popup menu (before tmux 3.5) a title row first.
	time.Sleep(500 * time.Millisecond)
	x, y = button("⋯")
	mark := c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "Show Hidden Files", 5*time.Second)
	time.Sleep(600 * time.Millisecond) // past the popup's first 400 ms
	item := y + 3
	if !h.menuMouse() {
		item++
	}
	c.Write(sgrMouse(35, x+4, item, 'M'))
	time.Sleep(100 * time.Millisecond)
	c.Click(0, x+4, item)
	h.waitFor("dot files shown", 5*time.Second, func() bool {
		s := h.screen(bar)
		return strings.Contains(s, "▸ 📁 .github") && strings.Contains(s, ".env")
	})
	if gitRow(h.screen(bar)) {
		t.Errorf(".git shown:\n%s", h.screen(bar))
	}
	h.waitFor("show_hidden saved", 5*time.Second, func() bool { return saved("true") })

	// A new tab's sidebar starts from the config: its explorer shows them.
	agentPane := ""
	for _, l := range strings.Split(h.mustTmux("list-panes", "-a", "-F", "#{pane_id} #{@imux_agent}"), "\n") {
		if f := strings.Fields(l); len(f) == 2 && f[1] == "claude" {
			agentPane = f[0]
		}
	}
	h.mustImux("tab", "codex", "--pane", agentPane)
	h.run("codex", 1)
	bar2 := ""
	h.waitFor("the new tab's sidebar", 10*time.Second, func() bool {
		for _, l := range strings.Split(h.mustTmux("list-panes", "-a", "-F", "#{pane_id} #{@imux_role}"), "\n") {
			if f := strings.Fields(l); len(f) == 2 && f[1] == "sidebar" && f[0] != bar {
				bar2 = f[0]
			}
		}
		return bar2 != ""
	})
	h.mustTmux("set-option", "-w", "-t", h.paneOpt(bar2, "#{window_id}"), "@imux_view", "explorer")
	h.mustImux("__sidebar-refresh")
	h.waitFor("the new tab's explorer with .env", 10*time.Second, func() bool { return strings.Contains(h.screen(bar2), ".env") })
	h.mustTmux("select-window", "-t", agentPane)

	// ⋯ → Go to File… (by its key) finds them too, and ⋯ has no Search…
	// (the rail has it); ⋯ shows ✓ now.
	more := func(key, want string) {
		t.Helper()
		time.Sleep(500 * time.Millisecond)
		x, y := button("⋯")
		mark := c.Mark()
		c.Click(0, x, y)
		c.waitScreenAfter(h, mark, "✓ Show Hidden Files", 5*time.Second)
		time.Sleep(600 * time.Millisecond)
		mark = c.Mark()
		c.Write(key)
		c.waitScreenAfter(h, mark, want, 5*time.Second)
		time.Sleep(300 * time.Millisecond)
	}
	more("p", "Go to File · proj")
	if strings.Contains(c.Text(), "Search…") {
		t.Error("⋯ has Search…")
	}
	mark = c.Mark()
	c.Write("ciyml")
	c.waitScreenAfter(h, mark, "1 files", 5*time.Second)
	c.Write("\x1b")

	// Show Hidden Files again (its key) hides them.
	more("h", "")
	h.waitFor("dot files hidden again", 5*time.Second, func() bool {
		s := h.screen(bar)
		return !strings.Contains(s, ".env") && !strings.Contains(s, ".github")
	})
	h.waitFor("show_hidden off", 5*time.Second, func() bool { return saved("false") })

	// New Folder…: the command prompt, then the folder, shown.
	time.Sleep(500 * time.Millisecond)
	x, y = button("📁")
	mark = c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "New folder name", 5*time.Second)
	c.Write("made\r")
	h.waitFor("the new folder", 5*time.Second, func() bool {
		fi, err := os.Stat(filepath.Join(p, "made"))
		return err == nil && fi.IsDir()
	})
	h.rowOf(bar, "📁 made")

	// Refresh: a file written elsewhere shows at once, not at the
	// sidebar's next gather (every 3 s while shown): right after one, the
	// file, then the click.
	pid, _ := strconv.Atoi(h.paneOpt(bar, "#{@imux_sidebar_pid}"))
	gathers := func() int {
		n := 0
		for _, r := range h.records() {
			if r.Pid == pid && slices.Contains(r.Args, "list-clients") {
				n++
			}
		}
		return n
	}
	x, y = button("🔄")
	time.Sleep(500 * time.Millisecond)
	n := gathers()
	h.waitFor("a gather", 5*time.Second, func() bool { return gathers() > n })
	testutil.WriteFile(t, filepath.Join(p, "zz-outside.txt"), []byte("x\n"))
	c.Click(0, x, y)
	h.waitFor("the new file", 1500*time.Millisecond, func() bool { return strings.Contains(h.screen(bar), "zz-outside.txt") })

	// Collapse Folders closes the open folders.
	time.Sleep(500 * time.Millisecond)
	x, y = h.rowOf(bar, "▸ 📁 src")
	c.Click(0, x, y)
	h.rowOf(bar, "▾ 📂 src")
	time.Sleep(500 * time.Millisecond)
	x, y = button("⊟")
	c.Click(0, x, y)
	h.waitFor("src closed", 5*time.Second, func() bool { return strings.Contains(h.screen(bar), "▸ 📁 src") })
}

// hoverAndSelection are the backgrounds of the row under the pointer and
// of the selected one in the default theme (D68), as capture-pane -e
// writes them: the hover halfway from the current row's to the selection's.
func hoverAndSelection() (hover, sel string) {
	p := theme.Builtin(theme.Default)
	bg := func(c theme.Color) string { return strings.TrimPrefix(c.Bg(), "\x1b[") }
	return bg(theme.HoverBg(p)), bg(p.SelectionBg)
}
