package workspace

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session/claude"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// writeAgy writes an Antigravity conversation of the project that started
// at started (its first step) and was last written at mtime.
func writeAgy(t *testing.T, h *testutil.Homes, id string, started, mtime time.Time) string {
	t.Helper()
	return testutil.WriteAntigravity(t, h, id, h.Project, mtime, testutil.AgyTurn{At: started, User: "task of " + id, Reply: "ok"})
}

func agyPane(id string, start time.Time, cwd string, extra ...TrailEntry) *Pane {
	tr := append(Trail{}, extra...)
	tr = append(tr, TrailEntry{Agent: "antigravity", Start: start.UnixMilli()})
	return &Pane{ID: id, Agent: "antigravity", CWD: cwd, Trail: tr}
}

func TestDiscoverMatchesRunsByStartTime(t *testing.T) {
	w, _, h := testWS(t)
	f := sources.New(config.Default())
	t0 := testutil.FrozenTime
	t1 := t0.Add(10 * time.Minute)
	// Both chats were written recently: the newest-by-mtime rule alone would
	// give pane A the chat of pane B.
	a := writeAgy(t, h, "aaaaaaaa-0000-4000-8000-000000000001", t0.Add(5*time.Second), t1.Add(21*time.Minute))
	b := writeAgy(t, h, "bbbbbbbb-0000-4000-8000-000000000002", t1.Add(5*time.Second), t1.Add(20*time.Minute))
	pa := agyPane("%1", t0, h.Project)
	pb := agyPane("%2", t1, h.Project)
	all := []*Pane{pa, pb}
	ctx := context.Background()
	if r, err := w.Discover(ctx, f, pa, all); err != nil || r.Path != a {
		t.Fatalf("pane A: %v %v, want %s", r.Path, err, a)
	}
	if r, err := w.Discover(ctx, f, pb, all); err != nil || r.Path != b {
		t.Fatalf("pane B: %v %v, want %s", r.Path, err, b)
	}
	// A pane in another directory does not compete.
	other := agyPane("%3", t1.Add(time.Minute), "/home/test/elsewhere")
	if r, err := w.Discover(ctx, f, pa, append(all, other)); err != nil || r.Path != a {
		t.Fatalf("with an unrelated pane: %v %v", r.Path, err)
	}
}

func TestDiscoverExcludesClaimedAndHonorsIDs(t *testing.T) {
	w, _, h := testWS(t)
	f := sources.New(config.Default())
	t0 := testutil.FrozenTime
	old := writeAgy(t, h, "cccccccc-0000-4000-8000-000000000003", t0.Add(time.Second), t0.Add(time.Hour))
	mine := writeAgy(t, h, "dddddddd-0000-4000-8000-000000000004", t0.Add(2*time.Second), t0.Add(30*time.Minute))
	// Another pane's closed run claims the newer file.
	z := &Pane{ID: "%9", Agent: "claude", CWD: h.Project, Trail: Trail{
		{Agent: "antigravity", Start: t0.UnixMilli(), End: t0.Add(time.Hour).UnixMilli(), File: old},
		{Agent: "claude", Start: t0.Add(time.Hour).UnixMilli()},
	}}
	pa := agyPane("%1", t0, h.Project)
	ctx := context.Background()
	if r, err := w.Discover(ctx, f, pa, []*Pane{pa, z}); err != nil || r.Path != mine {
		t.Fatalf("claimed file not excluded: %v %v", r.Path, err)
	}
	// A pre-assigned id wins over time-based discovery.
	id := "eeeeeeee-0000-4000-8000-000000000005"
	cp := filepath.Join(h.Claude, "projects", claude.EncodeCWD(h.Project), id+".jsonl")
	testutil.WriteFile(t, cp, []byte(`{"type":"user","sessionId":"`+id+`","cwd":"`+h.Project+`","timestamp":"2026-11-05T14:00:00Z","message":{"role":"user","content":"x"}}`+"\n"))
	testutil.Touch(t, cp, t0.Add(-24*time.Hour)) // even when older than the run
	pc := &Pane{ID: "%4", Agent: "claude", CWD: h.Project, Trail: Trail{{Agent: "claude", Start: t0.UnixMilli(), SessionID: id}}}
	if r, err := w.Discover(ctx, f, pc, []*Pane{pc}); err != nil || r.Path != cp || r.ID != id {
		t.Fatalf("pre-assigned id: %+v %v", r, err)
	}
	// Nothing written since the run started.
	late := agyPane("%5", t0.Add(2*time.Hour), h.Project)
	if _, err := w.Discover(ctx, f, late, []*Pane{late}); !errors.Is(err, sources.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	// A plain shell pane has no run.
	if _, err := w.Discover(ctx, f, &Pane{ID: "%6"}, nil); !errors.Is(err, sources.ErrNotFound) {
		t.Fatalf("shell pane: %v", err)
	}
}
