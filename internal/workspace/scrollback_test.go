package workspace

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func TestScrollbackSession(t *testing.T) {
	var b strings.Builder
	b.WriteString("\n\n")
	for i := 0; i < 200; i++ {
		b.WriteString("line ğüş ")
		b.WriteString(strings.Repeat("x", i%30))
		b.WriteString("   \n")
	}
	b.WriteString("last line on screen\n\n\n")
	s := ScrollbackSession("claude", "/home/test/proj", b.String(), testutil.FrozenTime)
	if s.Agent != "claude" || s.CWD != "/home/test/proj" || len(s.Warnings) != 1 {
		t.Fatalf("session %+v", s.Ref)
	}
	n := len(s.Entries)
	if n < 3 {
		t.Fatalf("expected several chunks, got %d", n)
	}
	last := s.Entries[n-1]
	if last.Role != session.RoleNotice || last.Text != ReconstructedNote {
		t.Fatalf("last entry %+v", last)
	}
	for _, e := range s.Entries[:n-1] {
		if e.Role != session.RoleAssistant || utf8.RuneCountInString(e.Text) > scrollbackChunk {
			t.Fatalf("chunk %d runes, role %v", utf8.RuneCountInString(e.Text), e.Role)
		}
		if strings.HasSuffix(e.Text, " ") || strings.HasPrefix(e.Text, "\n") {
			t.Fatalf("untrimmed chunk %q", e.Text)
		}
	}
	if !strings.HasSuffix(s.Entries[n-2].Text, "last line on screen") {
		t.Fatalf("newest chunk %q", s.Entries[n-2].Text)
	}
	if empty := ScrollbackSession("gemini", "/x", "\n  \n", testutil.FrozenTime); len(empty.Entries) != 0 {
		t.Fatalf("blank screen gave entries: %+v", empty.Entries)
	}
}
