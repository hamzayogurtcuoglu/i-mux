package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A short handoff message (§8.2b) is followed to its file: only a saved
// handoff file qualifies.
func TestParseChainedFollowsFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "handoffs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	doc := "[i-mux handoff · Claude Code → Codex CLI · 2026-11-07 09:47]\n\nintro\n\n## Original request\nbuild the thing\n## Notes from the user\nkept\n\n## Conversation (condensed)\n**User:** hi\n\n## What to do now\nwait\n"
	path := filepath.Join(dir, "20261107-094700-claude-to-codex.md")
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	msg := "[i-mux handoff · Claude Code → Codex CLI · 2026-11-07 09:47]\nWe continue here. Read this file.\n" + path
	c := ParseChained(msg)
	if c == nil || c.From != Claude || c.To != Codex || c.At.IsZero() {
		t.Fatalf("chained %+v", c)
	}
	if c.OriginalRequest != "build the thing\n## Notes from the user\nkept" || c.History != "**User:** hi" {
		t.Fatalf("from the file: %q / %q", c.OriginalRequest, c.History)
	}

	// Not a handoff file: nothing is read.
	other := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(other, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "20261107-094700-claude-to-codex.md")
	if err := os.WriteFile(outside, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{other, outside, "handoffs/20261107-094700-claude-to-codex.md", filepath.Join(dir, "20261107-094701-claude-to-codex.md")} {
		c := ParseChained(strings.Replace(msg, path, p, 1))
		if c == nil || c.OriginalRequest != "" || c.From != Claude {
			t.Fatalf("%s: %+v", p, c)
		}
	}
}
