//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestShortHandoffChain (T12.1, T12.2): with handoff.deliver = "file" the
// agent gets a three-line message (marker, sentence, path) instead of the
// document; the file holds the document; Claude also gets --add-dir for the
// handoffs directory. A second switch follows the message back to the file,
// so the original request survives Claude → Codex → Claude.
func TestShortHandoffChain(t *testing.T) {
	h := newH(t, "[handoff]\ndeliver = \"file\"\n")
	testutil.WriteFile(t, filepath.Join(h.Log, "codex.handoff-as-request"), nil)
	pane := h.startSession("claude")
	h.run("claude", 1)

	h.mustImux("switch", "codex", "--pane", pane)
	argv := h.run("codex", 1)
	if len(argv) != 1 {
		t.Fatalf("codex argv %q", argv)
	}
	lines := strings.Split(argv[0], "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "[i-mux handoff · Claude Code → Codex CLI · ") ||
		!strings.HasPrefix(lines[1], "We continue the Claude Code session here.") || filepath.Base(filepath.Dir(lines[2])) != "handoffs" {
		t.Fatalf("message %q", lines)
	}
	doc := h.readFile(lines[2])
	if !strings.Contains(doc, "## Original request") || !strings.Contains(doc, "fake claude request 1 from pane "+pane) {
		t.Fatalf("handoff file %.300q", doc)
	}

	h.mustImux("switch", "claude", "--pane", pane, "--no-resume")
	argv = h.run("claude", 2)
	if len(argv) != 4 || argv[0] != "--add-dir="+filepath.Dir(lines[2]) || argv[1] != "--session-id" {
		t.Fatalf("claude argv %q", argv)
	}
	second := strings.Split(argv[3], "\n")
	if len(second) != 3 || !strings.HasPrefix(second[0], "[i-mux handoff · Codex CLI → Claude Code") || second[2] == lines[2] {
		t.Fatalf("second message %q", second)
	}
	doc = h.readFile(second[2])
	orig, _, _ := strings.Cut(doc[strings.Index(doc, "## Original request"):], "\n## Earlier history")
	if !strings.Contains(orig, "fake claude request 1 from pane "+pane) || strings.Contains(orig, "We continue") {
		t.Fatalf("original request through the chain %.400q", orig)
	}
	if !strings.Contains(doc, "## Earlier history") || !strings.Contains(doc, "fake codex reply 1") {
		t.Fatalf("second handoff %.600q", doc)
	}
	if fi, err := os.Stat(second[2]); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("handoff file mode: %v %v", fi, err)
	}
}
