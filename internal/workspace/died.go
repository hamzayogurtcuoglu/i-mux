package workspace

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/launch"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// FastFail is how soon after its start an agent must die (with a non-zero
// status) for its fallback spec to be used (§9.5).
const FastFail = 10 * time.Second

// PaneDied handles the pane-died hook (remain-on-exit keeps the dead pane):
//
//   - a resumed run that failed fast (non-zero status within FastFail) is
//     replaced by its fallback spec (@imux_fallback: a new session with the
//     full handoff);
//   - otherwise the trail entry is closed, @imux_agent cleared, and the pane
//     drops to the login shell in @imux_cwd (or is killed with
//     on_exit = "close").
//
// status < 0 means unknown (killed by a signal).
func (w *WS) PaneDied(ctx context.Context, pane string, status int) error {
	defer w.RefreshSidebars(ctx)
	p, err := w.Pane(ctx, pane)
	if errors.Is(err, tmux.ErrNoPane) {
		return nil
	}
	if err != nil {
		return err
	}
	if !p.Dead {
		return nil // already respawned (a switch raced the hook)
	}
	now := w.Now()
	fast := status != 0 && !p.Start.IsZero() && now.Sub(p.Start) < FastFail
	if p.Fallback != "" {
		used, err := w.useFallback(ctx, p, fast)
		if used || err != nil {
			return err
		}
	}
	if p.Agent == "" && p.Trail.Last() == nil {
		// Not an imux run (a shell that kept remain-on-exit): close it.
		_, err := w.T.Run(ctx, tmux.Cmd{"kill-pane", "-t", p.ID})
		return err
	}
	trail := p.Trail.Close(now)
	msg := i18n.T("died.exited", session.ShortName(p.Agent), statusText(status))
	if fast {
		if l := p.Trail.Last(); l != nil && l.Handoff != "" {
			msg += " · " + i18n.T("died.handoff", paths.Shorten(l.Handoff))
		}
		msg += " · " + i18n.T("died.doctor")
	}
	if w.Cfg.OnExit == "close" {
		_, err := w.T.Run(ctx, tmux.Cmd{"kill-pane", "-t", p.ID})
		return err
	}
	// respawn-pane without a command would re-run the pane's original
	// command (imux __exec with a consumed spec): start the login shell.
	_, err = w.T.Run(ctx,
		tmux.Cmd{"respawn-pane", "-k", "-t", p.ID, "-c", p.CWD, w.loginShell(ctx), "-l"},
		tmux.Cmd{"set-option", "-p", "-t", p.ID, "remain-on-exit", "off"},
		tmux.Cmd{"set-option", "-p", "-u", "-t", p.ID, OptAgent},
		tmux.Cmd{"set-option", "-p", "-u", "-t", p.ID, OptStart},
		tmux.Cmd{"set-option", "-p", "-u", "-t", p.ID, OptLabel},
		tmux.Cmd{"set-option", "-p", "-u", "-t", p.ID, OptFlag},
		tmux.Cmd{"set-option", "-p", "-t", p.ID, OptCWD, p.CWD},
		tmux.Cmd{"set-option", "-p", "-t", p.ID, OptTrail, trail.String()},
	)
	if err != nil {
		return err
	}
	if p.Agent != "" {
		w.Message(ctx, "", p.ID, msg)
	}
	return nil
}

// useFallback respawns pane p from its fallback spec when the run failed
// fast; the spec file is removed either way.
func (w *WS) useFallback(ctx context.Context, p *Pane, fast bool) (bool, error) {
	fb := p.Fallback
	clear := tmux.Cmd{"set-option", "-p", "-u", "-t", p.ID, OptFallback}
	if !fast {
		os.Remove(fb)
		_, err := w.T.Run(ctx, clear)
		return false, err
	}
	spec, err := launch.Read(fb, true)
	if err != nil {
		_, _ = w.T.Run(ctx, clear)
		return false, nil // no usable fallback: fall through to the shell
	}
	spec.Pane = p.ID
	te := TrailEntry{Agent: spec.Agent, Start: w.Now().UnixMilli(), SessionID: argAfter(spec.Args, "--session-id")}
	trail := p.Trail
	if l := trail.Last(); l != nil {
		te.Handoff = l.Handoff
		trail = trail[:len(trail)-1] // the failed resume is replaced
	}
	trail = append(append(Trail{}, trail...), te)
	specPath, err := writeSpec(spec)
	if err != nil {
		return false, err
	}
	cmds := []tmux.Cmd{append(tmux.Cmd{"respawn-pane", "-k", "-t", p.ID, "-c", spec.Dir}, w.execCmd(specPath)...)}
	cmds = append(cmds, optCmds(p.ID, te, trail, p.CWD, w.Label(spec.Agent))...)
	cmds = append(cmds, clear)
	if _, err := w.T.Run(ctx, cmds...); err != nil {
		os.Remove(specPath)
		return false, err
	}
	w.Message(ctx, "", p.ID, i18n.T("died.fallback", session.ShortName(spec.Agent)))
	return true, nil
}

// loginShell returns tmux's default-shell (else $SHELL, else /bin/sh).
func (w *WS) loginShell(ctx context.Context) string {
	if out, err := w.T.Run(ctx, tmux.Cmd{"show-options", "-gv", "default-shell"}); err == nil {
		if sh := strings.TrimSpace(out); sh != "" {
			return sh
		}
	}
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	return "/bin/sh"
}

// SetFallback stores spec as pane's fallback (used by the resume path).
func (w *WS) SetFallback(ctx context.Context, pane string, spec *launch.Spec) error {
	p, err := writeSpec(spec)
	if err != nil {
		return err
	}
	_, err = w.T.Run(ctx, tmux.Cmd{"set-option", "-p", "-t", pane, OptFallback, p})
	if err != nil {
		os.Remove(p)
	}
	return err
}

func argAfter(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

func statusText(status int) string {
	if status < 0 {
		return "?"
	}
	return strconv.Itoa(status)
}
