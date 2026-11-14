package workspace

import (
	"context"
	"strconv"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agenthook"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// Options an agent's hooks write on its pane (D71): its state and when it
// said so (unix ms). The watcher takes them over its reading of the screen.
const (
	OptHook   = "@imux_hook"
	OptHookAt = "@imux_hook_at"
)

// ReportHook writes what agent's hook said on pane: its state, and for
// Claude the session it writes, into the pane's current trail entry. A
// pane that runs another agent (or none) is left alone, and so is the
// trail while a switch holds the pane.
func (w *WS) ReportHook(ctx context.Context, pane string, agent session.AgentID, r agenthook.Report) error {
	out, err := w.T.Run(ctx, tmux.Cmd{"display-message", "-p", "-t", pane, "#{" + OptAgent + "}\t#{" + OptLock + "}\t#{" + OptTrail + "}"})
	if err != nil {
		return err
	}
	f := strings.SplitN(strings.TrimRight(out, "\n"), "\t", 3)
	if len(f) < 3 || session.AgentID(f[0]) != agent {
		return nil
	}
	var cmds []tmux.Cmd
	if r.State != "" {
		cmds = append(cmds,
			tmux.Cmd{"set-option", "-p", "-q", "-t", pane, OptHook, r.State},
			tmux.Cmd{"set-option", "-p", "-q", "-t", pane, OptHookAt, strconv.FormatInt(w.Now().UnixMilli(), 10)})
	}
	if r.SessionID != "" && f[1] == "" {
		trail := ParseTrail(f[2])
		if l := trail.Last(); l != nil && l.Agent == agent && l.End == 0 && (l.SessionID != r.SessionID || r.File != "" && l.File != r.File) {
			// The session Claude writes now: after /clear, or a fork.
			l.SessionID = r.SessionID
			if r.File != "" {
				l.File = r.File
			}
			cmds = append(cmds, tmux.Cmd{"set-option", "-p", "-q", "-t", pane, OptTrail, trail.String()})
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	_, err = w.T.Run(ctx, cmds...)
	return err
}
