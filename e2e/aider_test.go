//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestAiderRoundTrip (T86.5, D81): Claude → Aider starts Aider with the
// handoff file as --read and no prompt; Aider first asks its .gitignore
// question, and the handoff is pasted only once that is answered. Back to
// Claude carries Aider's replies and edits from its chat history; a second
// switch to Aider restores its chat history.
func TestAiderRoundTrip(t *testing.T) {
	h := newH(t, "[handoff]\ndeliver = \"file\"\n")
	testutil.WriteFile(t, filepath.Join(h.Log, "aider.ask-gitignore"), nil)
	pane := h.startSession("claude")
	h.run("claude", 1)

	done := make(chan error, 1)
	go func() {
		_, _, err := h.imux("switch", "aider", "--pane", pane)
		done <- err
	}()
	argv := h.run("aider", 1)
	if len(argv) != 1 || !strings.HasPrefix(argv[0], "--read=") || !strings.Contains(argv[0], "/handoffs/") {
		t.Fatalf("aider argv %q", argv)
	}
	doc := h.readFile(strings.TrimPrefix(argv[0], "--read="))
	if !strings.HasPrefix(doc, "[i-mux handoff · Claude Code → Aider") || !strings.Contains(doc, "fake claude request 1") {
		t.Fatalf("handoff %.300q", doc)
	}
	h.waitFor("aider's question", 10*time.Second, func() bool { return strings.Contains(h.capture(pane), "(Y)es/(N)o") })
	// The screen is still and asks: nothing is pasted into the question.
	time.Sleep(3 * time.Second)
	if b, _ := os.ReadFile(filepath.Join(h.Log, "aider-1.answer")); len(b) != 0 {
		t.Fatalf("the handoff answered aider's question: %q", b)
	}
	h.mustTmux("send-keys", "-t", pane, "y", "Enter")
	h.waitFor("the handoff pasted after the answer", 30*time.Second, func() bool {
		return strings.Contains(h.stdinOf("aider", 1), "[i-mux handoff · Claude Code → Aider")
	})
	if got := strings.TrimSpace(h.readFile(filepath.Join(h.Log, "aider-1.answer"))); got != "y" {
		t.Fatalf("answer %q", got)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("switch: %v", err)
		}
	case <-time.After(15 * time.Second):
		out, _ := exec.Command("sh", "-c", "ps -o pid,ppid,command -U $USER | grep -E 'imux|fake' | grep -v grep").Output()
		t.Fatalf("switch did not return:\n%s", out)
	}
	if got := h.paneOpt(pane, "#{@imux_agent}|#{@imux_label}"); got != "aider|Aider" {
		t.Fatalf("pane %q", got)
	}
	hist := filepath.Join(h.Homes.Project, ".aider.chat.history.md")
	h.waitFor("aider's reply in its history", 10*time.Second, func() bool {
		return strings.Contains(h.readFile(hist), "fake aider reply 1")
	})

	h.mustImux("switch", "claude", "--pane", pane)
	h.waitDelivered("claude", 2, "fake aider reply 1", "edited fake1.go")

	h.mustImux("switch", "aider", "--pane", pane)
	argv = h.run("aider", 2)
	if len(argv) != 2 || !strings.HasPrefix(argv[0], "--read=") || argv[1] != "--restore-chat-history" {
		t.Fatalf("aider resume argv %q", argv)
	}
	h.waitFor("the delta pasted", 30*time.Second, func() bool {
		return strings.Contains(h.stdinOf("aider", 2), "[i-mux handoff · Claude Code → Aider")
	})
	h.waitDelivered("aider", 2, "fake claude reply 2")
}

// TestAiderSession (T86.3): an Aider run hands off from the History picker's
// form (`--session aider:<id>`), and the limits panel lists Aider.
func TestAiderSession(t *testing.T) {
	h := newH(t, "")
	hist := filepath.Join(h.Homes.Project, ".aider.chat.history.md")
	testutil.WriteFile(t, hist, []byte("\n# aider chat started at 2026-11-17 10:00:00\n\n> Aider v0.86.1  \n\n"+
		"#### make the parser faster  \n\nDone: I cached the regexps.\n\n> Applied edit to parse.go  \n"))
	doc := h.mustImux("handoff", "--session", "aider:20261117T100000", "--cwd", h.Homes.Project, "--print")
	for _, want := range []string{
		"[i-mux handoff · Aider", "make the parser faster", "cached the regexps", "edited parse.go",
		"the part that starts with `# aider chat started at 2026-11-17 10:00:00`",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("handoff lacks %q:\n%.900s", want, doc)
		}
	}
	out := h.mustImux("limits")
	if !strings.Contains(out, "Aider") || !strings.Contains(out, "Aider uses your model provider's limits") {
		t.Fatalf("limits:\n%s", out)
	}
}
