package workspace

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session/claude"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestPostResumeDiscovery (T6.4): a resumed run's trail entry records the
// session actually written to, whether the CLI appended or forked.
func TestPostResumeDiscovery(t *testing.T) {
	w, _, h := testWS(t)
	f := sources.New(config.Default())
	now := testutil.FrozenTime
	start := now.Add(-10 * time.Minute)
	dir := filepath.Join(h.Claude, "projects", claude.EncodeCWD(h.Project))
	write := func(id string, mtime time.Time) string {
		p := filepath.Join(dir, id+".jsonl")
		testutil.WriteFile(t, p, []byte(`{"type":"user","sessionId":"`+id+`","cwd":"`+jsonPath(h.Project)+`","timestamp":"2026-11-05T10:00:00Z","uuid":"u","message":{"role":"user","content":"x"}}`+"\n"))
		testutil.Touch(t, p, mtime)
		return p
	}
	orig := "aaaaaaaa-4444-4000-8000-000000000001"
	origPath := write(orig, now) // appended to during the run
	run := TrailEntry{Agent: "claude", Start: start.UnixMilli(), SessionID: orig, Resumed: true}
	p := &Pane{ID: "%1", Agent: "claude", CWD: h.Project, Trail: Trail{{Agent: "antigravity", Start: start.Add(-time.Hour).UnixMilli(), End: start.UnixMilli()}, run}}
	ctx := context.Background()

	// Appended: the resumed session itself.
	ref, err := w.Discover(ctx, f, p, []*Pane{p})
	if err != nil || ref.ID != orig || ref.Path != origPath {
		t.Fatalf("append: %+v %v", ref, err)
	}
	// An older unrelated session from before the run is not a fork.
	write("bbbbbbbb-4444-4000-8000-000000000002", start.Add(-time.Hour))
	if ref, _ := w.Discover(ctx, f, p, []*Pane{p}); ref.ID != orig {
		t.Fatalf("old session taken for a fork: %+v", ref)
	}
	// Forked: a new session started during the run.
	fork := "cccccccc-4444-4000-8000-000000000003"
	forkPath := write(fork, now.Add(time.Minute))
	ref, err = w.Discover(ctx, f, p, []*Pane{p})
	if err != nil || ref.ID != fork {
		t.Fatalf("fork: %+v %v", ref, err)
	}
	tr := recordSource(p.Trail, ref)
	if l := tr.Last(); l.SessionID != fork || l.File != forkPath || !l.Resumed {
		t.Fatalf("recorded %+v", l)
	}
	// A session claimed by another pane's run is not this run's fork.
	other := &Pane{ID: "%2", Agent: "claude", CWD: h.Project, Trail: Trail{{Agent: "claude", Start: start.UnixMilli(), SessionID: fork}}}
	if ref, _ := w.Discover(ctx, f, p, []*Pane{p, other}); ref.ID != orig {
		t.Fatalf("claimed session taken: %+v", ref)
	}
}
