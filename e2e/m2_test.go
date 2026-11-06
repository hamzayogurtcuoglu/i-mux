//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

// TestNoResidentProcesses (§17): __exec replaces itself with the agent; no
// ajan process stays per pane.
func TestNoResidentProcesses(t *testing.T) {
	h := newH(t, "")
	pane := h.startSession("claude")
	h.run("claude", 1)
	h.mustAjan("split", "gemini", "--pane", pane)
	h.run("gemini", 1)
	time.Sleep(300 * time.Millisecond)
	out, _ := exec.Command("pgrep", "-f", "^"+regexp.QuoteMeta(h.Bin)+" __exec").Output()
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Fatalf("resident ajan __exec processes: %s", s)
	}
	out, _ = exec.Command("pgrep", "-f", "^"+regexp.QuoteMeta(h.Bin)).Output()
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Fatalf("resident ajan processes: %s", s)
	}
}

// TestSwitchLatencyLargeSession (§17): switching away from a 64 MB Claude
// session takes < 1.5 s from the command to the new agent's exec.
func TestSwitchLatencyLargeSession(t *testing.T) {
	h := newH(t, "")
	pane := h.startSession("claude")
	h.run("claude", 1)
	trail := workspace.ParseTrail(h.paneOpt(pane, "#{@ajan_trail}"))
	b, err := os.ReadFile(filepath.Join(h.Log, "claude-1.session"))
	if err != nil {
		t.Fatal(err)
	}
	path := strings.TrimSpace(string(b))
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := testutil.GenClaudeSession(f, 64<<20, h.Homes.Project, trail[0].SessionID); err != nil {
		t.Fatal(err)
	}
	f.Close()
	start := time.Now()
	h.mustAjan("switch", "gemini", "--pane", pane)
	argv := h.run("gemini", 1)
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
		mark := c.Mark()
		c.Write(keyCtrlBackslash)
		c.waitScreenAfter(h, mark, "ajan · ", 5*time.Second)
		return c.Mark()
	}

	// Sessions…
	mark := open()
	c.Write("s")
	c.waitScreenAfter(h, mark, "Sessions · ", 5*time.Second)
	c.Write("q")
	time.Sleep(100 * time.Millisecond)

	// New pane → Gemini (uppercase mnemonic).
	open()
	c.Write("G")
	h.run("gemini", 1)
	h.waitFor("two panes", 5*time.Second, func() bool {
		return strings.Count(h.mustTmux("list-panes", "-t", pane, "-F", "#{pane_id}"), "%") == 2
	})
	h.mustTmux("select-pane", "-t", pane)
	time.Sleep(100 * time.Millisecond)

	// Preview handoff: a popup with the document.
	mark = open()
	c.Write("v")
	c.waitScreenAfter(h, mark, "[ajan handoff · Claude Code →", 5*time.Second)
	c.Write("q") // quit less; the popup closes
	time.Sleep(200 * time.Millisecond)

	// Limits: coming soon until M4.
	mark = open()
	c.Write("l")
	c.waitScreenAfter(h, mark, "limits panel", 5*time.Second)
}
