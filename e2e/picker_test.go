//go:build e2e

package e2e

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

// TestSessionPicker (T2.8c): the picker lists conversations of all installed
// agents for the pane's directory, newest first, 9 per page with paging; a
// picked conversation continues in another agent or natively; opening it is
// fast with 50 candidate files.
func TestSessionPicker(t *testing.T) {
	h := newH(t, "")
	now := time.Now()
	agents := []string{"claude", "gemini", "copilot", "codex"}
	ids := map[string]string{}
	for i := 0; i < 50; i++ {
		a := agents[i%4]
		id := h.synthetic(a, i+1, fmt.Sprintf("synthetic %s task %02d", a, i), now.Add(-time.Duration(i+2)*time.Minute))
		if ids[a] == "" {
			ids[a] = id
		}
	}
	pane := h.startSession("claude")
	h.run("claude", 1)
	c, client := h.attach(140, 40)

	h.mustAjan("pick", "--pane", pane)
	c.waitScreen(h, "Sessions · ", 5*time.Second)
	c.waitScreen(h, "synthetic claude task 00", 2*time.Second)
	c.waitScreen(h, "synthetic gemini task 01", 2*time.Second)
	c.waitScreen(h, "More…", 2*time.Second)
	mark := c.Mark()
	c.Write("m")
	c.waitScreenAfter(h, mark, "(2/", 5*time.Second)
	// Row → second-level menu.
	mark = c.Mark()
	c.Write("1")
	c.waitScreenAfter(h, mark, "(with context)", 5*time.Second)
	c.Write("q")

	// Continue a picked Codex conversation in Gemini, with context from it.
	h.mustAjan("switch", "gemini", "--pane", pane, "--from-session", "codex:"+ids["codex"])
	argv := h.run("gemini", 1)
	if len(argv) != 2 || !strings.HasPrefix(argv[1], "[ajan handoff · Codex CLI → Gemini CLI") ||
		!strings.Contains(argv[1], "synthetic codex task 03") || strings.Contains(argv[1], "fake claude request") {
		t.Fatalf("handoff not built from the picked conversation:\n%.600s", strings.Join(argv, " "))
	}
	// Continue a picked Codex conversation natively.
	h.mustAjan("switch", "codex", "--pane", pane, "--from-session", "codex:"+ids["codex"])
	if argv := h.run("codex", 1); len(argv) != 2 || argv[0] != "resume" || argv[1] != ids["codex"] {
		t.Fatalf("native resume argv %q", argv)
	}
	trail := workspace.ParseTrail(h.paneOpt(pane, "#{@ajan_trail}"))
	if l := trail.Last(); l.Agent != "codex" || !l.Resumed || l.SessionID != ids["codex"] {
		t.Fatalf("trail after resume %+v", l)
	}
	// "… in a new pane": a split continues the picked Claude conversation.
	np := strings.TrimSpace(h.mustAjan("split", "gemini", "-h", "--pane", pane, "--from-session", "claude:"+ids["claude"]))
	argv = h.run("gemini", 2)
	if !strings.Contains(strings.Join(argv, " "), "synthetic claude task 00") || h.paneOpt(np, "#{@ajan_agent}") != "gemini" {
		t.Fatalf("split from session: pane %s argv %.300q", np, argv)
	}

	// Latency with 50 candidate files (§17: < 150 ms p95).
	var ds []time.Duration
	for i := 0; i < 50; i++ {
		start := time.Now()
		h.mustAjan("__menu", "--sessions", "--pane", pane, "--client", client)
		ds = append(ds, time.Since(start))
		c.Write("q")
		time.Sleep(20 * time.Millisecond)
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	p95 := ds[len(ds)*95/100-1]
	t.Logf("picker open latency: p50 %v, p95 %v, max %v", ds[len(ds)/2], p95, ds[len(ds)-1])
	if p95 > 150*time.Millisecond {
		t.Errorf("picker p95 %v exceeds 150 ms", p95)
	}
}
