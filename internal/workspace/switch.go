package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff"
	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff/clean"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// ErrBusy means another switch of the same pane is in progress.
var ErrBusy = errors.New("switch in progress")

// paneLock serializes switches of one pane: an flock (atomic, released when
// the process dies) plus @imux_lock with our pid, which other invocations
// check for liveness.
type paneLock struct {
	f    *os.File
	w    *WS
	pane string
}

func lockPath(pane string) string {
	return filepath.Join(paths.Home(), "locks", "pane-"+strings.TrimPrefix(pane, "%")+".lock")
}

// lockPane takes the pane's switch lock or returns ErrBusy.
func (w *WS) lockPane(ctx context.Context, p *Pane) (*paneLock, error) {
	if pid, err := strconv.Atoi(p.Lock); err == nil && pid > 0 && pid != os.Getpid() && alive(pid) {
		return nil, ErrBusy
	}
	if err := paths.EnsureDir(filepath.Dir(lockPath(p.ID))); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(lockPath(p.ID), os.O_RDWR|os.O_CREATE, paths.FileMode)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, ErrBusy
	}
	if _, err := w.T.Run(ctx, tmux.Cmd{"set-option", "-p", "-t", p.ID, OptLock, strconv.Itoa(os.Getpid())}); err != nil {
		f.Close()
		return nil, err
	}
	return &paneLock{f: f, w: w, pane: p.ID}, nil
}

func (l *paneLock) release(ctx context.Context) {
	_, _ = l.w.T.Run(ctx, tmux.Cmd{"set-option", "-p", "-u", "-t", l.pane, OptLock})
	l.f.Close()
}

func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// SwitchReq is one `imux switch`.
type SwitchReq struct {
	Pane     string
	To       session.AgentID
	Reason   string       // "", "manual" or "limit"
	NoResume bool         // never resume natively (M6)
	From     *session.Ref // a conversation picked in the session picker
	Client   string       // client to show messages on ("" = tmux picks)
	// NewPane opens the target in a new pane split from Pane (side by side)
	// instead of replacing Pane's agent. Used with From.
	NewPane bool
}

// SwitchResult says what happened.
type SwitchResult struct {
	Pane     string
	From, To session.AgentID
	Handoff  string // handoff file ("" when started fresh)
	Runes    int
	Fresh    bool // no previous conversation: started without context
	Resumed  bool // a picked conversation continued natively in its own agent
	Pasted   bool // the handoff was pasted into the agent (inject = "paste")
	Delta    bool // Resumed with a delta handoff of what happened elsewhere
	// Reconstructed: no session file; the context came from the scrollback.
	Reconstructed bool
	Warnings      []string
}

// Switch moves pane req.Pane to agent req.To with the conversation as
// context (§9.5): the source is parsed and the handoff file written before
// the old agent is killed, so any failure leaves it running.
func (w *WS) Switch(ctx context.Context, f *sources.Finder, req SwitchReq) (*SwitchResult, error) {
	if !w.Installed(req.To) {
		return nil, &NotInstalledError{Agent: req.To}
	}
	p, err := w.Pane(ctx, req.Pane)
	if err != nil {
		return nil, err
	}
	if !req.NewPane {
		lock, err := w.lockPane(ctx, p)
		if err != nil {
			return nil, err
		}
		defer lock.release(ctx)
	}

	all, err := w.Panes(ctx)
	if err != nil {
		return nil, err
	}
	src := p
	if p.Agent == "" || p.Dead {
		src = shellSource(p) // context from the pane's last run, if any
	}
	res := &SwitchResult{Pane: p.ID, From: src.Agent, To: req.To}
	var reason *handoff.Reason
	switch req.Reason {
	case "limit":
		reason = &handoff.Reason{Limit: true}
		if l := w.State().ActiveLimit(string(src.Agent), w.Now()); l != nil {
			reason.ResetAt = l.ResetAt
		}
	case "manual":
		reason = &handoff.Reason{}
	}
	l := Launch{Agent: req.To, CWD: p.CWD, Pane: p.ID}
	trail := p.Trail
	var built *HandoffResult
	native := req.From != nil && req.From.Agent == req.To
	if native {
		l.ResumeID, res.Resumed = req.From.ID, true // continue it natively
		if req.From.CWD != "" {
			l.CWD = req.From.CWD
		}
	} else if src.Agent != "" || req.From != nil {
		built, err = w.BuildHandoff(ctx, f, HandoffReq{Pane: src, All: all, To: req.To, Reason: reason, From: req.From})
		switch {
		case errors.Is(err, ErrNoSource) && req.From == nil && p.Agent == "":
			built = nil // a shell pane whose old run left nothing: start fresh
		case err != nil:
			return nil, err
		}
	}
	if built != nil {
		res.From = built.From
		res.Warnings = built.Warnings
		res.Reconstructed = built.Reconstructed
		path, err := handoff.Save(built.Doc.Text, built.From, req.To, w.Now())
		if err != nil {
			return nil, err
		}
		res.Handoff, res.Runes = path, built.Doc.Runes
		l.Prompt, l.Handoff = w.firstMessage(built.Doc.Text, path, built.From, req.To, built.Limit, false), path
		if req.From == nil && built.Ref.Path != "" {
			trail = recordSource(trail, built.Ref)
		}
	} else if !native {
		res.Fresh = true
	}
	// Returning to an agent that already ran in this pane resumes its session
	// natively with a delta handoff; the full handoff becomes the fallback
	// for a resume that fails fast (§8.6, §9.5).
	fallback := ""
	if !native && !req.NoResume && !req.NewPane && req.From == nil {
		if r, ok := w.FindResumable(ctx, f, p, req.To); ok {
			var cur *session.Ref
			if built != nil && built.Ref.Path != "" {
				cur = &built.Ref
			}
			delta, _, err := w.BuildDelta(ctx, f, p, r, req.To, cur)
			if err != nil {
				return nil, err
			}
			if built != nil {
				spec, _, err := w.Spec(Launch{Agent: req.To, CWD: p.CWD, Prompt: w.firstMessage(built.Doc.Text, res.Handoff, built.From, req.To, built.Limit, false), Handoff: res.Handoff, Pane: p.ID})
				if err != nil {
					return nil, err
				}
				if fallback, err = writeSpec(spec); err != nil {
					return nil, err
				}
			}
			l.ResumeID, l.Prompt, l.Handoff = r.Ref.ID, "", ""
			res.Resumed, res.Fresh, res.Runes = true, false, 0
			if delta != nil {
				path, err := handoff.Save(delta.Text, res.From, req.To, w.Now())
				if err != nil {
					return nil, err
				}
				l.Prompt, l.Handoff = w.firstMessage(delta.Text, path, res.From, req.To, false, true), path
				res.Handoff, res.Runes, res.Delta = path, delta.Runes, true
			}
		}
	}
	target := p.ID
	if req.NewPane {
		l.Pane = ""
		np, err := w.launchNew(ctx, l, func(cmd []string) tmux.Cmd {
			return append(tmux.Cmd{"split-window", "-h", "-t", p.ID, "-c", l.CWD, "-P", "-F", "#{pane_id}"}, cmd...)
		})
		if err != nil {
			return nil, err
		}
		target, res.Pane = np, np
	} else if err := w.respawn(ctx, p, l, trail, fallback); err != nil {
		if fallback != "" {
			os.Remove(fallback)
		}
		return nil, err
	}
	w.notifySwitch(ctx, req.Client, target, res)
	if w.pasteMode(l) {
		res.Pasted = true
		if err := w.WaitStable(ctx, target); err != nil {
			return res, err
		}
		if err := w.Paste(ctx, target, l.Prompt, true); err != nil {
			return res, err
		}
	}
	return res, nil
}

// shellSource turns a plain-shell pane into the pane its last run was in.
func shellSource(p *Pane) *Pane {
	last := p.Trail.Last()
	if last == nil {
		return &Pane{ID: p.ID, CWD: p.CWD}
	}
	q := *p
	q.Agent = last.Agent
	q.Dead = false
	return &q
}

// recordSource stores the discovered session in the run's trail entry so that
// other panes never claim it (for a resumed run, the session it actually
// wrote to, whether the CLI appended or forked).
func recordSource(t Trail, ref session.Ref) Trail {
	out := append(Trail{}, t...)
	if l := out.Last(); l != nil && l.Agent == ref.Agent {
		if l.Resumed {
			// The session actually written to (a fork has a new id).
			l.File, l.SessionID = ref.Path, ref.ID
			return out
		}
		if l.File == "" {
			l.File = ref.Path
		}
		if l.SessionID == "" {
			l.SessionID = ref.ID
		}
	}
	return out
}

// respawn replaces the pane's process with run l and updates its options.
// fallback (a spec file) becomes @imux_fallback in the same exec, so a run
// that dies at once cannot race it; "" clears it.
func (w *WS) respawn(ctx context.Context, p *Pane, l Launch, trail Trail, fallback string) error {
	spec, te, err := w.Spec(l)
	if err != nil {
		return err
	}
	specPath, err := writeSpec(spec)
	if err != nil {
		return err
	}
	trail = trail.Push(te, w.Now())
	cmds := []tmux.Cmd{append(tmux.Cmd{"respawn-pane", "-k", "-t", p.ID, "-c", l.CWD}, w.execCmd(specPath)...)}
	cmds = append(cmds, optCmds(p.ID, te, trail, l.CWD, w.Label(l.Agent))...)
	if fallback != "" {
		cmds = append(cmds, tmux.Cmd{"set-option", "-p", "-t", p.ID, OptFallback, fallback})
	} else {
		cmds = append(cmds, tmux.Cmd{"set-option", "-p", "-u", "-t", p.ID, OptFallback})
	}
	if _, err := w.T.Run(ctx, cmds...); err != nil {
		os.Remove(specPath)
		return err
	}
	return nil
}

// notifySwitch shows the result on the client (and the one-time
// cross-provider notice on the first switch).
func (w *WS) notifySwitch(ctx context.Context, client, pane string, r *SwitchResult) {
	w.RefreshSidebars(ctx)
	var msg string
	switch {
	case r.Resumed && r.Delta:
		msg = i18n.T("switch.resumed_delta", session.ShortName(r.To), clean.Thousands(r.Runes))
	case r.Resumed:
		msg = i18n.T("switch.resumed", session.ShortName(r.To))
	case r.Fresh:
		msg = i18n.T("switch.fresh", session.ShortName(r.To))
	default:
		msg = i18n.T("switch.done", session.ShortName(r.To), clean.Thousands(r.Runes))
	}
	switch {
	case r.Reconstructed:
		msg += " · " + i18n.T("switch.reconstructed")
	case len(r.Warnings) > 0:
		msg += " · " + i18n.T("switch.warnings", len(r.Warnings))
	}
	if r.Handoff != "" && !w.State().Notices["cross_provider_ack"] {
		msg += " · " + i18n.T("notice.cross_provider", session.DisplayName(r.From), session.DisplayName(r.To))
		_ = state.Update(func(s *state.State) error { s.SetNotice("cross_provider_ack"); return nil })
	}
	w.Message(ctx, client, pane, msg)
}

// Message shows msg on client (or the client tmux picks for pane's session).
func (w *WS) Message(ctx context.Context, client, pane, msg string) {
	c := tmux.Cmd{"display-message"}
	if client != "" {
		c = append(c, "-c", client)
	}
	if pane != "" {
		c = append(c, "-t", pane)
	}
	_, err := w.T.Run(ctx, append(c, tmux.EscapeMessage(msg)))
	if err != nil && client != "" {
		// The client may have gone; let tmux pick one.
		_, _ = w.T.Run(ctx, tmux.Cmd{"display-message", "-t", pane, tmux.EscapeMessage(msg)})
	}
}

// SwitchError formats a failed switch for display-message.
func SwitchError(to session.AgentID, err error) string {
	if errors.Is(err, ErrBusy) {
		return i18n.T("switch.busy")
	}
	var ni *NotInstalledError
	if errors.As(err, &ni) {
		return err.Error()
	}
	return i18n.T("switch.failed", session.ShortName(to), err.Error())
}

// firstMessage is what the new agent receives for a saved handoff: a short
// message pointing to its file (handoff.deliver = "file", §8.2b), or the
// document itself ("inline").
func (w *WS) firstMessage(doc, path string, from, to session.AgentID, limit, delta bool) string {
	if w.Cfg.Handoff.Deliver == "inline" || path == "" {
		return doc
	}
	return handoff.Message{From: from, To: to, At: w.Now(), Path: path, Limit: limit, Delta: delta}.Text()
}
