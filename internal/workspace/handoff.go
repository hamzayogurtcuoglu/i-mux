package workspace

import (
	"context"
	"errors"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
)

// HandoffReq describes a handoff built for a pane.
type HandoffReq struct {
	Pane   *Pane
	All    []*Pane // every pane (for discovery); nil = read them
	To     session.AgentID
	Reason *handoff.Reason // nil = derived from the source session
	Budget int             // 0 = config
	// From, when set, is the conversation picked in the session picker; it
	// replaces the pane's current one as the source.
	From *session.Ref
}

// HandoffResult is a built handoff and where it came from.
type HandoffResult struct {
	Doc      *handoff.Doc
	From     session.AgentID
	Ref      session.Ref // zero when reconstructed from scrollback
	Source   *session.Session
	Warnings []string
	// Reconstructed is set when no session file was found and the pane's
	// scrollback was used instead.
	Reconstructed bool
	Limit         bool // the source hit its usage limit
}

// ErrNoSource means neither a session file nor the scrollback had anything.
var ErrNoSource = errors.New("no source session")

// BuildHandoff loads the source conversation of a pane (or the picked one)
// and renders the handoff for req.To.
func (w *WS) BuildHandoff(ctx context.Context, f *sources.Finder, req HandoffReq) (*HandoffResult, error) {
	p := req.Pane
	var ref session.Ref
	var err error
	if req.From != nil {
		ref = *req.From
	} else {
		all := req.All
		if all == nil {
			if all, err = w.Panes(ctx); err != nil {
				return nil, err
			}
		}
		ref, err = w.Discover(ctx, f, p, all)
	}
	res := &HandoffResult{}
	var s *session.Session
	switch {
	case err == nil:
		s, err = f.Load(ctx, ref, timeZero)
		if err != nil {
			return nil, err
		}
		res.Ref = ref
	case errors.Is(err, sources.ErrNotFound):
		s, err = w.scrollbackSession(ctx, p)
		if err != nil {
			return nil, err
		}
		res.Reconstructed = true
		if len(s.Entries) == 0 && p.Agent == "" {
			return nil, ErrNoSource // a plain shell with an empty screen: nothing to carry over
		}
	default:
		return nil, err
	}
	res.Source = s
	res.From = s.Agent
	res.Warnings = s.Warnings
	reason := sources.LimitReason(s)
	if req.Reason != nil {
		reason = *req.Reason
	}
	if reason.Limit && reason.ResetAt.IsZero() {
		// The watcher may have seen the reset time on the screen.
		if l := w.State().ActiveLimit(string(s.Agent), w.Now()); l != nil {
			reason.ResetAt = l.ResetAt
		}
	}
	cwd := p.CWD
	if req.From != nil && s.CWD != "" {
		cwd = s.CWD
	}
	doc, err := f.Build(ctx, sources.BuildRequest{
		Sources: []*session.Session{s}, From: s.Agent, To: req.To, CWD: cwd,
		Reason: reason, Budget: req.Budget, Now: w.Now(),
	})
	if err != nil {
		return nil, err
	}
	res.Doc, res.Limit = doc, reason.Limit
	return res, nil
}

// scrollbackSession is the fallback when no session file is found (T2.11).
var scrollbackFallback = func(w *WS, ctx context.Context, p *Pane) (*session.Session, error) {
	return nil, ErrNoSource
}

func (w *WS) scrollbackSession(ctx context.Context, p *Pane) (*session.Session, error) {
	return scrollbackFallback(w, ctx, p)
}

var timeZero = time.Time{}
