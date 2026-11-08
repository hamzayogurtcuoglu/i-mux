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
)

// TestSourceControl (T22.5): in the 🔀 view a click on a file's ➕ stages
// it, Commit… asks for a message and commits, ⏪ asks and discards a new file.
func TestSourceControl(t *testing.T) {
	h := newH(t, sidebarConfig)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
		cmd.Dir = h.Homes.Project
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	testutil.WriteFile(t, filepath.Join(h.Homes.Project, "a.txt"), []byte("one\n"))
	git("init", "-q", "-b", "main")
	git("add", "-A")
	git("commit", "-q", "-m", "init")
	testutil.WriteFile(t, filepath.Join(h.Homes.Project, "a.txt"), []byte("one\ntwo\n"))
	testutil.WriteFile(t, filepath.Join(h.Homes.Project, "new.txt"), []byte("x\n"))
	// Commits made from the sidebar need an identity.
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")

	_, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	x, y := h.rowOf(bar, "🔀")
	c.Click(0, x, y)
	h.rowOf(bar, "CHANGES  2")
	// col is the client column fromEnd columns from the sidebar's right edge.
	col := func(fromEnd int) int {
		f := strings.Fields(h.mustTmux("display-message", "-p", "-t", bar, "#{pane_left} #{pane_width}"))
		left, _ := strconv.Atoi(f[0])
		w, _ := strconv.Atoi(f[1])
		return left + w - fromEnd
	}

	// + on a.txt stages it.
	// A click while the sidebar redraws after attaching can be lost: retry.
	for attempt := 0; ; attempt++ {
		time.Sleep(500 * time.Millisecond)
		_, y = h.rowOf(bar, "M  a.txt")
		c.Click(0, col(2), y) // the last button: ➕ (" ➕ ", 4 columns)
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) && !strings.Contains(h.screen(bar), "STAGED CHANGES  1") {
			time.Sleep(100 * time.Millisecond)
		}
		if strings.Contains(h.screen(bar), "STAGED CHANGES  1") || attempt == 2 {
			break
		}
	}
	h.rowOf(bar, "STAGED CHANGES  1")
	if out := git("diff", "--cached", "--name-only"); strings.TrimSpace(out) != "a.txt" {
		t.Fatalf("staged %q", out)
	}

	// Commit… → message → committed (only the staged file).
	time.Sleep(500 * time.Millisecond)
	x, y = h.rowOf(bar, "Commit…")
	mark := c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "Commit message:", 5*time.Second)
	c.Write("add two\r")
	h.waitFor("commit", 5*time.Second, func() bool {
		return strings.Contains(git("log", "--oneline", "-n", "1"), "add two")
	})
	h.rowOf(bar, "add two") // in HISTORY

	// ↶ on new.txt asks, then deletes it.
	time.Sleep(500 * time.Millisecond)
	_, y = h.rowOf(bar, "?  new.txt")
	mark = c.Mark()
	c.Click(0, col(6), y) // ⏪, left of ➕
	c.waitScreenAfter(h, mark, "cannot be undone", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	c.Write("y")
	h.waitFor("new.txt discarded", 5*time.Second, func() bool {
		_, err := os.Stat(filepath.Join(h.Homes.Project, "new.txt"))
		return os.IsNotExist(err)
	})
}
