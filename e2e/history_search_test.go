//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session/claude"
)

// TestHistorySearch (T51.1): History's last item searches the conversations'
// text in a popup: a word said later in one of them (not its first request)
// finds it, with the message; Enter opens that conversation's menu.
func TestHistorySearch(t *testing.T) {
	h := newH(t, "")
	now := time.Now()
	for i := 0; i < 6; i++ {
		h.synthetic("claude", i+1, fmt.Sprintf("synthetic claude task %02d", i), now.Add(-time.Duration(i+2)*time.Minute))
	}
	h.synthetic("codex", 9, "synthetic codex task", now.Add(-time.Hour))
	// Conversation 3 talks about a zebra later on.
	id := fmt.Sprintf("%08x-5e55-4000-8000-%012d", 3, 3)
	p := filepath.Join(h.Homes.Claude, "projects", claude.EncodeCWD(h.Homes.Project), id+".jsonl")
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	ts := now.Add(-4 * time.Minute).UTC().Format(time.RFC3339)
	fmt.Fprintf(f, `{"type":"user","sessionId":%q,"cwd":%q,"timestamp":%q,"uuid":"u2","message":{"role":"user","content":"and the zebra crossing on the login page is broken"}}`+"\n", id, h.Homes.Project, ts)
	_ = f.Close()
	if err := os.Chtimes(p, now.Add(-4*time.Minute), now.Add(-4*time.Minute)); err != nil {
		t.Fatal(err)
	}

	pane := h.startSession("claude")
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	h.mustImux("pick", "--pane", pane)
	c.waitScreen(h, "Sessions · ", 5*time.Second)
	mark := c.Mark()
	c.Write("/")
	c.waitScreenAfter(h, mark, "History · ", 5*time.Second)
	c.waitScreenAfter(h, mark, "synthetic codex task", 5*time.Second) // all of them, listed
	mark = c.Mark()
	c.Write("zebra")
	c.waitScreenAfter(h, mark, "1 found", 10*time.Second)
	c.waitScreenAfter(h, mark, "synthetic claude task 02", 5*time.Second) // its first request names it
	c.waitScreenAfter(h, mark, "you  and the zebra crossing", 5*time.Second)
	if out := stripANSI(c.Output()[mark:]); strings.Contains(out, "2 found") {
		t.Fatal("more than one conversation found")
	}
	mark = c.Mark()
	c.Write("\r")
	c.waitScreenAfter(h, mark, "(with context)", 5*time.Second) // its menu
	c.Write("q")
}
