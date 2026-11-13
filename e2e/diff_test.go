//go:build e2e

package e2e

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestDiffMarks (T69.2): the diff viewer marks a file's changes beside its
// scroll bar; ] goes to the next change and [ back, the bottom row saying
// which; a click on the mark of the last change goes there.
func TestDiffMarks(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	h := newH(t, sidebarConfig)
	p := h.Homes.Project
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
		cmd.Dir = p
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	file := func(change map[int]string) {
		var b strings.Builder
		for i := 1; i <= 300; i++ {
			l := "line " + strconv.Itoa(i)
			if s, ok := change[i]; ok {
				l = s
			}
			b.WriteString(l + "\n")
		}
		testutil.WriteFile(t, filepath.Join(p, "long.txt"), []byte(b.String()))
	}
	file(nil)
	git("init", "-q", "-b", "main")
	git("add", "-A")
	git("commit", "-q", "-m", "init")
	file(map[int]string{30: "line 30 CHANGED", 250: "line 250 CHANGED"})

	agentPane, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	h.rowOf(bar, "AGENTS") // the sidebar has its width: the pane will not move
	h.mustTmux("respawn-pane", "-k", "-t", agentPane, "-c", p, h.Bin+" __diff M long.txt")
	shows := func(what string, want ...string) {
		t.Helper()
		h.waitFor(what, 5*time.Second, func() bool {
			s := h.screen(agentPane)
			for _, w := range want {
				if !strings.Contains(s, w) {
					return false
				}
			}
			return true
		})
	}
	shows("the diff, at its first change", "long.txt", "+2 −2", "line 30 CHANGED", "next and previous change")

	// ] and [: the next change and back.
	h.mustTmux("send-keys", "-t", agentPane, "]")
	shows("the second change", "line 250 CHANGED", "change 2 of 2")
	h.mustTmux("send-keys", "-t", agentPane, "[")
	shows("the first change again", "line 30 CHANGED", "change 1 of 2")

	// Back to the top, then a click on the last mark beside the bar.
	h.mustTmux("send-keys", "-t", agentPane, "g")
	shows("the top", "line 1 ")
	x, y := 0, 0
	h.waitFor("the marks beside the bar", 5*time.Second, func() bool {
		rows := strings.Split(h.screen(agentPane), "\n")
		for i := len(rows) - 1; i >= 0 && x == 0; i-- {
			r := []rune(rows[i])
			if len(r) < 3 {
				continue
			}
			for k := len(r) - 3; k < len(r)-1; k++ {
				if r[k] == '▐' || r[k] == '▌' {
					x, y = sidebar.DisplayWidth(string(r[:k]))+1, i+1
					break
				}
			}
		}
		return x > 0
	})
	pos := strings.Fields(h.mustTmux("display-message", "-p", "-t", agentPane, "#{pane_left} #{pane_top} #{status-position} #{status}"))
	left, _ := strconv.Atoi(pos[0])
	top, _ := strconv.Atoi(pos[1])
	if pos[2] == "top" && pos[3] != "off" {
		top++ // the tab bar takes the client's first line
	}
	// A click right after a client attached can be lost (see openView):
	// back to the top and again.
	jumped := func() bool {
		s := h.screen(agentPane)
		return strings.Contains(s, "line 250 CHANGED") && strings.Contains(s, "change 2 of 2")
	}
	for attempt := 0; !jumped(); attempt++ {
		if attempt == 3 {
			t.Fatalf("a click on the mark at %d,%d did not go to the change:\n%s", left+x, top+y, h.screen(agentPane))
		}
		h.mustTmux("send-keys", "-t", agentPane, "g")
		time.Sleep(500 * time.Millisecond) // past the double-click window
		c.Click(0, left+x, top+y)
		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline) && !jumped(); {
			time.Sleep(100 * time.Millisecond)
		}
	}
	h.mustTmux("send-keys", "-t", agentPane, "q")
}
