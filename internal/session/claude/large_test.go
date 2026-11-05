package claude

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

const largeID = "99999999-0000-4000-8000-000000000064"

func largeFixture(tb testing.TB, size int64) session.Ref {
	tb.Helper()
	dir := tb.TempDir()
	p := filepath.Join(dir, largeID+".jsonl")
	f, err := os.Create(p)
	if err != nil {
		tb.Fatal(err)
	}
	if err := testutil.GenClaudeSession(f, size, "/home/test/proj", largeID); err != nil {
		tb.Fatal(err)
	}
	f.Close()
	return session.Ref{Agent: session.Claude, ID: largeID, Path: p}
}

func TestLargeSessionLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("large fixture")
	}
	ref := largeFixture(t, 64<<20)
	s, err := New(session.Env{}).Load(context.Background(), ref, session.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Gap || len(s.Entries) < 100 || s.FirstUserText() == "" || len(s.Todos) != 2 {
		t.Fatalf("gap=%v entries=%d todos=%d", s.Gap, len(s.Entries), len(s.Todos))
	}
}

func BenchmarkLoadLarge64MB(b *testing.B) {
	ref := largeFixture(b, 64<<20)
	p := New(session.Env{})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := p.Load(context.Background(), ref, session.LoadOptions{}); err != nil {
			b.Fatal(err)
		}
	}
}
