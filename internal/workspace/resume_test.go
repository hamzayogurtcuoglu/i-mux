package workspace

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/launch"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
)

func paneRow(p *Pane) string {
	return strings.Join([]string{p.ID, string(p.Agent), "1762300000000", p.CWD, p.CWD, "", "0", "proj", "@1", "node", "", "", p.Trail.String()}, "\t") + "\n"
}

// TestSwitchResumesWithDelta (T6.3): switching back to an agent that ran in
// the pane resumes its session with a delta; the full handoff is the
// fallback; --no-resume starts a new session.
func TestSwitchResumesWithDelta(t *testing.T) {
	w, f, h := testWS(t)
	w.SetBins(map[session.AgentID]agent.Resolved{
		"claude": {Agent: "claude", Installed: true, Path: "/opt/bin/claude", PathEnv: "/opt/bin"},
		"codex":  {Agent: "codex", Installed: true, Path: "/opt/bin/codex", PathEnv: "/opt/bin"},
	})
	tr, _ := multiHop(t, h)
	p := &Pane{ID: "%1", Agent: "codex", CWD: h.Project, Trail: tr}
	f.replies["display-message"] = paneRow(p)
	f.replies["list-panes"] = paneRow(p)
	finder := sources.New(config.Default())
	res, err := w.Switch(context.Background(), finder, SwitchReq{Pane: "%1", To: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Resumed || !res.Delta || res.Runes == 0 {
		t.Fatalf("result %+v", res)
	}
	var respawn []string
	for _, c := range f.calls {
		if slices.Contains(c, "respawn-pane") {
			respawn = c
		}
	}
	joined := strings.Join(respawn, " ")
	var spec, fallback string
	for i, a := range respawn {
		if a == "__exec" {
			spec = respawn[i+1]
		}
		if a == OptFallback && i+1 < len(respawn) {
			fallback = respawn[i+1]
		}
	}
	if spec == "" || fallback == "" || strings.Contains(joined, "-u -t %1 "+OptFallback) {
		t.Fatalf("respawn batch lacks the spec or the fallback: %s", joined)
	}
	s, err := launch.Read(spec, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Args) != 3 || s.Args[0] != "--resume" || s.Args[1] != claudeA || !strings.Contains(s.Args[2], "Welcome back") ||
		!strings.Contains(s.Args[2], "codex wrote the tests") || strings.Contains(s.Args[2], "claude original task") {
		t.Fatalf("resume argv %.300q", s.Args)
	}
	fb, err := launch.Read(fallback, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(fb.Args) != 3 || fb.Args[0] != "--session-id" || !strings.HasPrefix(fb.Args[2], "[ajan handoff · Codex CLI → Claude Code") || fb.Pane != "%1" {
		t.Fatalf("fallback argv %.200q", fb.Args)
	}
	if !strings.Contains(joined, `"r":true`) || !strings.Contains(joined, `"id":"`+claudeA+`"`) {
		t.Fatalf("trail entry of the resume: %s", joined)
	}

	// --no-resume: a new session with the full handoff, no fallback.
	f.calls = nil
	res, err = w.Switch(context.Background(), finder, SwitchReq{Pane: "%1", To: "claude", NoResume: true})
	if err != nil || res.Resumed {
		t.Fatalf("no-resume: %+v %v", res, err)
	}
	for _, c := range f.calls {
		if slices.Contains(c, "respawn-pane") {
			if !strings.Contains(strings.Join(c, " "), "-u -t %1 "+OptFallback) {
				t.Fatalf("fallback not cleared: %q", c)
			}
		}
	}
}
