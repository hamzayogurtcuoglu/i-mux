package workspace

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
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

// An Antigravity pane has no readable session file: the handoff comes from
// the scrollback, plus the task list Antigravity keeps in brain/<id>/task.md
// (only one written during this run).
func TestScrollbackAntigravityTaskList(t *testing.T) {
	w, f, h := testWS(t)
	f.replies["capture-pane"] = "> fix the login redirect\nWorking on src/auth.ts ...\n"
	start := testutil.FrozenTime
	task := filepath.Join(h.Home, ".gemini", "antigravity-cli", "brain", "aaaaaaaa-1", "task.md")
	testutil.WriteFile(t, task, []byte("- [x] Read src/auth.ts\n- [/] Fix the redirect\n"))
	if err := os.Chtimes(task, start.Add(time.Minute), start.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	p := &Pane{
		ID: "%1", Agent: session.Antigravity, CWD: h.Project,
		Trail: Trail{{Agent: session.Antigravity, Start: start.UnixMilli()}},
	}
	s, err := w.scrollbackSession(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Agent != session.Antigravity || len(s.Entries) == 0 || !strings.Contains(s.Entries[0].Text, "fix the login redirect") {
		t.Fatalf("session %+v", s)
	}
	want := []session.Todo{{Text: "Read src/auth.ts", Status: session.TodoDone}, {Text: "Fix the redirect", Status: session.TodoInProgress}}
	if !reflect.DeepEqual(s.Todos, want) {
		t.Fatalf("todos %+v", s.Todos)
	}
	// A plan from before this run is not this conversation's.
	p.Trail[0].Start = start.Add(time.Hour).UnixMilli()
	if s, _ = w.scrollbackSession(context.Background(), p); s.Todos != nil {
		t.Fatalf("stale plan attached: %+v", s.Todos)
	}
	// Other agents never get Antigravity's plan.
	p.Agent, p.Trail[0].Agent, p.Trail[0].Start = "claude", "claude", start.UnixMilli()
	if s, _ = w.scrollbackSession(context.Background(), p); s.Todos != nil {
		t.Fatalf("claude got antigravity's plan: %+v", s.Todos)
	}
}
