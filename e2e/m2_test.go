//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

// TestNoResidentProcesses (§17): __exec replaces itself with the agent; no
// imux process stays per pane.
func TestNoResidentProcesses(t *testing.T) {
	h := newH(t, "")
	pane := h.startSession("claude")
	h.run("claude", 1)
	h.mustImux("split", "antigravity", "--pane", pane)
	h.run("antigravity", 1)
	time.Sleep(300 * time.Millisecond)
	// The watcher is the only resident imux process (§2); no __exec stays.
	// ps, not pgrep -a: on macOS -a means "include ancestors".
	out, err := exec.Command("ps", "-A", "-o", "args=").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(string(out), "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, h.Bin+" ") && !strings.HasSuffix(l, " __watch run") {
			t.Fatalf("resident imux process: %s", l)
		}
	}
}

// TestSwitchLatencyLargeSession (§17): switching away from a 64 MB Claude
// session takes < 1.5 s from the command to the new agent's exec.
func TestSwitchLatencyLargeSession(t *testing.T) {
	h := newH(t, "")
	pane := h.startSession("claude")
	h.run("claude", 1)
	trail := workspace.ParseTrail(h.paneOpt(pane, "#{@imux_trail}"))
	f, err := os.Create(h.sessionPath("claude", 1))
	if err != nil {
		t.Fatal(err)
	}
	if err := testutil.GenClaudeSession(f, 64<<20, h.Homes.Project, trail[0].SessionID); err != nil {
		t.Fatal(err)
	}
	f.Close()
	start := time.Now()
	h.mustImux("switch", "antigravity", "--pane", pane)
	argv := h.run("antigravity", 1)
	d := time.Since(start)
	t.Logf("switch from a 64 MB session: %v, handoff %d bytes", d, len(argv[len(argv)-1]))
	if d > 1500*time.Millisecond {
		t.Errorf("switch took %v, budget 1.5 s", d)
	}
}

// TestMenuItemsByKey (T2.14): the remaining pane-menu items work through
// their mnemonics on a real client.
func TestMenuItemsByKey(t *testing.T) {
	h := newH(t, "")
	pane := h.startSession("claude")
	h.run("claude", 1)
	c, _ := h.attach(uiCols, uiRows)
	c.waitScreen(h, "fake claude ready", 5*time.Second)
	open := func() int {
		t.Helper()
		c.openWith(h, keyCtrlBackslash, "i-mux · ")
		return c.Mark()
	}

	// Sessions…
	mark := open()
	c.Write("s")
	c.waitScreenAfter(h, mark, "Sessions · ", 5*time.Second)
	c.Write("q")
	time.Sleep(100 * time.Millisecond)

	// New pane → Antigravity (uppercase mnemonic).
	open()
	c.Write("A")
	h.run("antigravity", 1)
	h.waitFor("two panes", 5*time.Second, func() bool {
		return strings.Count(h.mustTmux("list-panes", "-t", pane, "-F", "#{pane_id}"), "%") == 2
	})
	h.mustTmux("select-pane", "-t", pane)
	time.Sleep(100 * time.Millisecond)

	// Preview handoff: a popup with the document.
	mark = open()
	c.Write("v")
	c.waitScreenAfter(h, mark, "[i-mux handoff · Claude Code →", 5*time.Second)
	c.Write("q") // quit less; the popup closes
	time.Sleep(200 * time.Millisecond)

	// Limits: the panel in a popup.
	mark = open()
	c.Write("l")
	c.waitScreenAfter(h, mark, "USAGE (local data)", 5*time.Second)
	c.Write("q")
}
