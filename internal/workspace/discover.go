package workspace

import (
	"context"
	"os"
	"sort"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
)

// discoverSlack absorbs timestamp precision (some CLIs write whole seconds)
// when comparing a run's start with a session's start or mtime.
const discoverSlack = 1500 * time.Millisecond

// Discover finds the session file of pane p's current run (§9.4, T2.5):
//
//  1. the file or id already recorded in the trail (a pre-assigned Claude
//     --session-id, a resumed session);
//  2. otherwise the agent's sessions for the pane's directory that were
//     written since the run started, minus files and ids claimed by any
//     trail entry of another run;
//  3. when other panes run the same agent in the same directory, runs and
//     files are matched by start time: newest run first, each taking the
//     earliest session that started after it.
//
// It returns sources.ErrNotFound when nothing matches.
func (w *WS) Discover(ctx context.Context, f *sources.Finder, p *Pane, all []*Pane) (session.Ref, error) {
	cur := p.Trail.Last()
	if p.Agent == "" || cur == nil || cur.Agent != p.Agent {
		return session.Ref{}, sources.ErrNotFound
	}
	prov := f.Provider(cur.Agent)
	if prov == nil {
		return session.Ref{}, sources.ErrNotFound
	}
	if cur.Resumed && cur.File == "" {
		// A resumed CLI either appends to the session it resumed or forks
		// a new one (T6.4): a session of this agent that started during
		// this run, and that no other trail claims, is the fork.
		if ref, ok := w.forkOf(ctx, prov, p, cur, all); ok {
			return ref, nil
		}
	}
	if cur.File != "" {
		if fi, err := os.Stat(cur.File); err == nil {
			if cur.SessionID != "" {
				if refs, _ := prov.Find(ctx, session.Query{ID: cur.SessionID, CWD: p.CWD}); len(refs) > 0 {
					return refs[0], nil
				}
			}
			ref, err := f.Resolve(ctx, cur.Agent, cur.File, p.CWD)
			if err == nil {
				ref.UpdatedAt, ref.Size = fi.ModTime(), fi.Size()
				return ref, nil
			}
		}
	}
	if cur.SessionID != "" {
		if refs, _ := prov.Find(ctx, session.Query{ID: cur.SessionID, CWD: p.CWD}); len(refs) > 0 {
			return refs[0], nil
		}
		// Nothing written under the pre-assigned id (yet, or the CLI
		// ignored it): fall back to time-based discovery.
	}

	// Runs competing for the same files: this one plus the current runs of
	// other live panes with the same agent and directory whose file is not
	// known yet.
	type run struct {
		pane  string
		start time.Time
	}
	runs := []run{{p.ID, cur.StartTime()}}
	claimedFile := map[string]bool{}
	claimedID := map[string]bool{}
	for _, q := range all {
		for i, e := range q.Trail {
			if q.ID == p.ID && i == len(q.Trail)-1 {
				continue // the run being discovered
			}
			if e.Agent != cur.Agent {
				continue
			}
			if e.File != "" {
				claimedFile[e.File] = true
			}
			if e.SessionID != "" {
				claimedID[e.SessionID] = true
			}
			if q.ID != p.ID && i == len(q.Trail)-1 && e.End == 0 && e.File == "" && e.SessionID == "" &&
				!q.Dead && q.Agent == cur.Agent && session.SameDir(q.CWD, p.CWD) {
				runs = append(runs, run{q.ID, e.StartTime()})
			}
		}
	}
	since := cur.StartTime()
	for _, r := range runs {
		if r.start.Before(since) {
			since = r.start
		}
	}
	refs, err := prov.Find(ctx, session.Query{CWD: p.CWD, Since: since.Add(-discoverSlack)})
	if err != nil {
		return session.Ref{}, err
	}
	var cands []session.Ref
	for _, r := range refs {
		if !claimedFile[r.Path] && !claimedID[r.ID] {
			cands = append(cands, r)
		}
	}
	if len(cands) == 0 {
		return session.Ref{}, sources.ErrNotFound
	}
	if len(runs) == 1 {
		// Alone: the newest file written since the run started.
		for _, c := range cands {
			if !c.UpdatedAt.Before(cur.StartTime().Add(-discoverSlack)) {
				return c, nil
			}
		}
		return session.Ref{}, sources.ErrNotFound
	}
	key := func(r session.Ref) time.Time {
		if !r.StartedAt.IsZero() {
			return r.StartedAt
		}
		return r.UpdatedAt
	}
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].start.After(runs[j].start) })
	sort.SliceStable(cands, func(i, j int) bool { return key(cands[i]).Before(key(cands[j])) })
	taken := make([]bool, len(cands))
	for _, r := range runs {
		for i, c := range cands {
			if taken[i] || key(c).Before(r.start.Add(-discoverSlack)) {
				continue
			}
			taken[i] = true
			if r.pane == p.ID {
				return c, nil
			}
			break
		}
	}
	return session.Ref{}, sources.ErrNotFound
}

// forkOf looks for a new session started by a resumed run: same agent and
// directory, a different id, started (or, without a start time, written)
// after the run began, and not claimed by any trail entry.
func (w *WS) forkOf(ctx context.Context, prov session.Provider, p *Pane, cur *TrailEntry, all []*Pane) (session.Ref, bool) {
	start := cur.StartTime().Add(-discoverSlack)
	refs, err := prov.Find(ctx, session.Query{CWD: p.CWD, Since: start})
	if err != nil {
		return session.Ref{}, false
	}
	claimed := map[string]bool{cur.SessionID: true}
	for _, q := range all {
		for _, e := range q.Trail {
			if e.Agent == cur.Agent && e.SessionID != "" {
				claimed[e.SessionID] = true
			}
		}
	}
	for _, r := range refs {
		if claimed[r.ID] {
			continue
		}
		t := r.StartedAt
		if t.IsZero() {
			t = r.UpdatedAt
		}
		if !t.Before(start) {
			return r, true
		}
	}
	return session.Ref{}, false
}
