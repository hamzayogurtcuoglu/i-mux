package workspace

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session/claude"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestReasonFromSessionError (T3.7): a manual switch away from an agent whose
// last API error is limit-like says "hit its usage limit" in the handoff.
func TestReasonFromSessionError(t *testing.T) {
	w, _, h := testWS(t)
	loc := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = loc })
	f := sources.New(config.Default())
	id := "aaaaaaaa-7777-4000-8000-000000000001"
	p := filepath.Join(h.Claude, "projects", claude.EncodeCWD(h.Project), id+".jsonl")
	reset := time.Date(2026, 11, 5, 17, 0, 0, 0, time.UTC)
	lines := `{"type":"user","sessionId":"` + id + `","cwd":"` + h.Project + `","timestamp":"2026-11-05T14:00:00Z","uuid":"u","message":{"role":"user","content":"refactor the cache"}}
{"type":"assistant","sessionId":"` + id + `","timestamp":"2026-11-05T14:01:00Z","uuid":"a","message":{"id":"m1","role":"assistant","model":"claude-opus-4-1","content":[{"type":"text","text":"Working on it."}]}}
{"type":"assistant","isApiErrorMessage":true,"error":"rate_limit","sessionId":"` + id + `","timestamp":"2026-11-05T14:02:00Z","uuid":"e","message":{"id":"x","role":"assistant","model":"<synthetic>","content":[{"type":"text","text":"Claude AI usage limit reached|` + itoa(int(reset.Unix())) + `"}]}}
`
	testutil.WriteFile(t, p, []byte(lines))
	pane := &Pane{ID: "%1", Agent: "claude", CWD: h.Project, Trail: Trail{{Agent: "claude", Start: testutil.FrozenTime.Add(-time.Hour).UnixMilli(), SessionID: id}}}
	res, err := w.BuildHandoff(context.Background(), f, HandoffReq{Pane: pane, All: []*Pane{pane}, To: "gemini"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Doc.Text, "because Claude Code hit its usage limit (resets 17:00)") {
		t.Fatalf("reason line missing:\n%.600s", res.Doc.Text)
	}
	// An explicit manual reason wins.
	res, _ = w.BuildHandoff(context.Background(), f, HandoffReq{Pane: pane, All: []*Pane{pane}, To: "gemini", Reason: &handoff.Reason{}})
	if !strings.Contains(res.Doc.Text, "switched to you manually") {
		t.Fatal("explicit manual reason ignored")
	}
	// Without a reset in the error, the watcher's record supplies it.
	lines = strings.Replace(lines, "Claude AI usage limit reached|"+itoa(int(reset.Unix())), "Rate limit reached", 1)
	testutil.WriteFile(t, p, []byte(lines))
	w.st = &state.State{Limits: map[string]*state.Limit{"claude": {At: testutil.FrozenTime, ResetAt: reset.Add(time.Hour), ExpiresAt: reset.Add(time.Hour), Severity: "limit"}}}
	res, _ = w.BuildHandoff(context.Background(), f, HandoffReq{Pane: pane, All: []*Pane{pane}, To: "gemini"})
	if !strings.Contains(res.Doc.Text, "hit its usage limit (resets 18:00)") {
		t.Fatalf("reset from state missing:\n%.600s", res.Doc.Text)
	}
}
