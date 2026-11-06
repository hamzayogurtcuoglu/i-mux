package workspace

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session/claude"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

const (
	claudeA = "aaaaaaaa-6666-4000-8000-000000000001"
	agyG    = "bbbbbbbb-6666-4000-8000-000000000002"
	codexX  = "cccccccc-6666-4000-8000-000000000003"
)

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// multiHop writes claude(A, t0–t1) → antigravity(G, t1–t2) → codex(X, t2–now).
func multiHop(t *testing.T, h *testutil.Homes) (Trail, [3]time.Time) {
	t.Helper()
	now := testutil.FrozenTime
	t0, t1, t2 := now.Add(-3*time.Hour), now.Add(-2*time.Hour), now.Add(-time.Hour)
	cp := filepath.Join(h.Claude, "projects", claude.EncodeCWD(h.Project), claudeA+".jsonl")
	testutil.WriteFile(t, cp, []byte(fmt.Sprintf(`{"type":"user","sessionId":%q,"cwd":%q,"timestamp":%q,"uuid":"u","message":{"role":"user","content":"claude original task"}}
{"type":"assistant","sessionId":%q,"timestamp":%q,"uuid":"a","message":{"id":"m1","role":"assistant","model":"m","content":[{"type":"text","text":"claude did part one"}]}}
`, claudeA, h.Project, ts(t0), claudeA, ts(t0.Add(time.Minute)))))
	testutil.Touch(t, cp, t1)
	gp := testutil.WriteAntigravity(t, h, agyG, h.Project, t2,
		testutil.AgyTurn{At: t1.Add(time.Minute), User: "antigravity side quest", Reply: "antigravity fixed the parser"})
	xp := filepath.Join(h.Codex, "sessions", "2026", "11", "05", "rollout-2026-11-05T13-32-00-"+codexX+".jsonl")
	testutil.WriteFile(t, xp, []byte(fmt.Sprintf(`{"timestamp":%q,"type":"session_meta","payload":{"id":%q,"cwd":%q}}
{"timestamp":%q,"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"codex wrote the tests"}]}}
`, ts(t2), codexX, h.Project, ts(t2.Add(time.Minute)))))
	testutil.Touch(t, xp, now)
	tr := Trail{
		{Agent: "claude", Start: t0.UnixMilli(), End: t1.UnixMilli(), SessionID: claudeA, File: cp},
		{Agent: "antigravity", Start: t1.UnixMilli(), End: t2.UnixMilli(), File: gp},
		{Agent: "codex", Start: t2.UnixMilli()},
	}
	return tr, [3]time.Time{t0, t1, t2}
}

func TestDeltaOverMultiHopTrail(t *testing.T) {
	w, _, h := testWS(t)
	w.SetBins(map[session.AgentID]agent.Resolved{
		"claude": {Agent: "claude", Installed: true, Path: "/opt/bin/claude"},
		"codex":  {Agent: "codex", Installed: true, Path: "/opt/bin/codex"},
	})
	f := sources.New(config.Default())
	tr, _ := multiHop(t, h)
	p := &Pane{ID: "%1", Agent: "codex", CWD: h.Project, Trail: tr}
	ctx := context.Background()
	r, ok := w.FindResumable(ctx, f, p, "claude")
	if !ok || r.Prior.SessionID != claudeA || len(r.After) != 2 {
		t.Fatalf("resumable %+v %v", r, ok)
	}
	doc, srcs, err := w.BuildDelta(ctx, f, p, r, "claude", nil)
	if err != nil || doc == nil || len(srcs) != 2 {
		t.Fatalf("delta: %v %v", doc, err)
	}
	for _, want := range []string{"Welcome back", "antigravity side quest", "antigravity fixed the parser", "codex wrote the tests"} {
		if !strings.Contains(doc.Text, want) {
			t.Errorf("delta lacks %q:\n%s", want, doc.Text)
		}
	}
	for _, bad := range []string{"claude original task", "claude did part one", "## Original request"} {
		if strings.Contains(doc.Text, bad) {
			t.Errorf("delta contains %q", bad)
		}
	}
	// Not resumable: the current run, an agent without a run here, resume
	// disabled, or a session file that is gone.
	if _, ok := w.FindResumable(ctx, f, p, "codex"); ok {
		t.Error("the current run is not a resume")
	}
	if _, ok := w.FindResumable(ctx, f, p, "copilot"); ok {
		t.Error("no earlier copilot run")
	}
	w.Cfg.Agents["claude"].Resume = false
	if _, ok := w.FindResumable(ctx, f, p, "claude"); ok {
		t.Error("resume = false must disable it")
	}
	w.Cfg.Agents["claude"].Resume = true
	gone := p.Trail[0]
	gone.SessionID, gone.File = "dddddddd-0000-4000-8000-000000000000", ""
	if _, ok := w.FindResumable(ctx, f, &Pane{ID: "%1", Agent: "codex", CWD: h.Project, Trail: Trail{gone, tr[1], tr[2]}}, "claude"); ok {
		t.Error("missing session file must not resume")
	}
}

func TestDeltaEmpty(t *testing.T) {
	w, _, h := testWS(t)
	f := sources.New(config.Default())
	tr, times := multiHop(t, h)
	// Claude → Antigravity, but Antigravity wrote nothing after Claude's run ended.
	g := tr[1]
	g.File, g.End = "", 0
	g.Start = times[2].Add(30 * time.Minute).UnixMilli()
	p := &Pane{ID: "%1", Agent: "antigravity", CWD: h.Project, Trail: Trail{tr[0], g}}
	r, ok := w.FindResumable(context.Background(), f, p, "claude")
	if !ok {
		t.Fatal("not resumable")
	}
	r.Prior.End = testutil.FrozenTime.UnixMilli() // everything happened before
	doc, _, err := w.BuildDelta(context.Background(), f, p, r, "claude", nil)
	if err != nil || doc != nil {
		t.Fatalf("empty delta should give no document: %v %v", doc, err)
	}
}
