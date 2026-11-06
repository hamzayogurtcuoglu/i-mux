package workspace

import (
	"context"
	"errors"
	"os"

	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
)

// RefFor finds the session of one trail entry: its recorded file, its
// session id, else the newest file of that agent for cwd written during the
// run (mtime ≥ start, started before the run ended).
func (w *WS) RefFor(ctx context.Context, f *sources.Finder, e TrailEntry, cwd string) (session.Ref, error) {
	prov := f.Provider(e.Agent)
	if prov == nil {
		return session.Ref{}, sources.ErrNotFound
	}
	if e.File != "" {
		if _, err := os.Stat(e.File); err == nil {
			if e.SessionID != "" {
				if refs, _ := prov.Find(ctx, session.Query{ID: e.SessionID, CWD: cwd}); len(refs) > 0 && refs[0].Path == e.File {
					return refs[0], nil
				}
			}
			return f.Resolve(ctx, e.Agent, e.File, cwd)
		}
	}
	if e.SessionID != "" {
		if refs, _ := prov.Find(ctx, session.Query{ID: e.SessionID, CWD: cwd}); len(refs) > 0 {
			return refs[0], nil
		}
	}
	refs, err := prov.Find(ctx, session.Query{CWD: cwd, Since: e.StartTime().Add(-discoverSlack)})
	if err != nil {
		return session.Ref{}, err
	}
	end := e.EndTime()
	for _, r := range refs {
		if !end.IsZero() && !r.StartedAt.IsZero() && r.StartedAt.After(end.Add(discoverSlack)) {
			continue // a later run's session
		}
		return r, nil
	}
	return session.Ref{}, sources.ErrNotFound
}

// Resumable is a native resume of an earlier run in the pane (§8.6).
type Resumable struct {
	Prior TrailEntry  // the agent's last run in this pane
	Ref   session.Ref // its session (exists)
	After []TrailEntry
}

// FindResumable reports whether switching pane p to agent `to` can resume
// the agent's earlier session natively: the trail has a run of `to` with a
// session id whose file still exists (in the pane's directory), resume is
// enabled for the agent and its template has a resume form.
func (w *WS) FindResumable(ctx context.Context, f *sources.Finder, p *Pane, to session.AgentID) (*Resumable, bool) {
	ac := w.Cfg.Agent(string(to))
	if !ac.Resume || len(ac.ResumeArgs) == 0 {
		return nil, false
	}
	idx := -1
	for i := len(p.Trail) - 1; i >= 0; i-- {
		if p.Trail[i].Agent == to {
			idx = i
			break
		}
	}
	if idx < 0 || idx == len(p.Trail)-1 && p.Agent == to && !p.Dead {
		return nil, false // never ran here, or it is the current run
	}
	prior := p.Trail[idx]
	if prior.SessionID == "" && prior.File == "" {
		return nil, false
	}
	ref, err := w.RefFor(ctx, f, prior, p.CWD)
	if err != nil || ref.ID == "" || !session.SameDir(firstNonEmpty(ref.CWD, p.CWD), p.CWD) {
		return nil, false
	}
	if prior.SessionID != "" && ref.ID != prior.SessionID && (prior.File == "" || ref.Path != prior.File) {
		return nil, false // only the recorded session is resumed, never a look-alike
	}
	if _, err := os.Stat(ref.Path); err != nil {
		return nil, false
	}
	return &Resumable{Prior: prior, Ref: ref, After: append([]TrailEntry{}, p.Trail[idx+1:]...)}, true
}

// BuildDelta renders the "welcome back" handoff for a resume: every run
// after the prior one, oldest first, each loaded with the entries since
// that run started (never before the prior run's end, with slack for
// CLIs that write whole-second timestamps). current is the pane's current
// run's session when known (it was discovered for the switch). It returns
// nil when nothing happened
// elsewhere (resume without a prompt).
func (w *WS) BuildDelta(ctx context.Context, f *sources.Finder, p *Pane, r *Resumable, to session.AgentID, current *session.Ref) (*handoff.Doc, []*session.Session, error) {
	priorEnd := r.Prior.EndTime()
	var srcs []*session.Session
	entries := 0
	var from session.AgentID
	for i, e := range r.After {
		var ref session.Ref
		var err error
		if i == len(r.After)-1 && current != nil && current.Path != "" && e.Agent == current.Agent {
			ref = *current
		} else {
			ref, err = w.RefFor(ctx, f, e, p.CWD)
		}
		if errors.Is(err, sources.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		// Only what happened during that run: a run that itself resumed an
		// older session must not carry that session's earlier history.
		since := e.StartTime().Add(-discoverSlack)
		if since.Before(priorEnd.Add(-discoverSlack)) {
			since = priorEnd.Add(-discoverSlack)
		}
		s, err := f.Load(ctx, ref, since)
		if err != nil {
			continue
		}
		srcs = append(srcs, s)
		entries += len(s.Entries)
		from = s.Agent
	}
	if entries == 0 {
		return nil, srcs, nil
	}
	doc, err := f.Build(ctx, sources.BuildRequest{
		Sources: srcs, From: from, To: to, CWD: p.CWD, Delta: true, Now: w.Now(),
		Budget: w.deltaBudget(),
	})
	return doc, srcs, err
}

// deltaBudget is the character budget of a delta handoff: the full budget
// (it only carries what happened elsewhere, so it is usually much smaller).
func (w *WS) deltaBudget() int { return w.Cfg.Handoff.BudgetChars }

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
