package watch

import (
	"context"
	"sort"
	"strconv"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// WorkingFor is how long after its last screen change a pane counts as
// working.
const WorkingFor = 3 * time.Second

// PaneStates writes each agent pane's state (@imux_state: working, idle or
// blocked) from the watcher's activity, only when it changes, with the time
// of the change (@imux_state_at, unix seconds) and, when work ended, the
// time it ended (@imux_done_at: the sidebar shows "Done" until the user looks
// at the pane). Then it makes the sidebars redraw (§9.3 "Sidebar v4").
type PaneStates struct {
	T    *tmux.Client
	last map[string]string
}

// Commands returns the set-option commands for states that changed. An idle
// screen that asks a question is blocked ("Needs you").
func (s *PaneStates) Commands(at time.Time, changed map[string]time.Time, asking map[string]bool) []tmux.Cmd {
	if s.last == nil {
		s.last = map[string]string{}
	}
	panes := make([]string, 0, len(changed))
	for p := range changed {
		panes = append(panes, p)
	}
	sort.Strings(panes)
	var cmds []tmux.Cmd
	seen := map[string]bool{}
	stamp := strconv.FormatInt(at.Unix(), 10)
	for _, p := range panes {
		seen[p] = true
		st := sidebar.StateIdle
		switch {
		case at.Sub(changed[p]) < WorkingFor:
			st = sidebar.StateWorking
		case asking[p]:
			st = sidebar.StateBlocked
		}
		prev := s.last[p]
		if prev == st {
			continue
		}
		s.last[p] = st
		set := func(k, v string) tmux.Cmd { return tmux.Cmd{"set-option", "-p", "-q", "-t", p, k, v} }
		cmds = append(cmds, set(sidebar.OptState, st), set(sidebar.OptStateAt, stamp))
		if prev == sidebar.StateWorking && st == sidebar.StateIdle {
			cmds = append(cmds, set(sidebar.OptDoneAt, stamp))
		}
	}
	for p := range s.last {
		if !seen[p] {
			delete(s.last, p) // the pane is gone
		}
	}
	return cmds
}

// Apply writes the changed states and signals the sidebars.
func (s *PaneStates) Apply(ctx context.Context, at time.Time, changed map[string]time.Time, asking map[string]bool) {
	cmds := s.Commands(at, changed, asking)
	if len(cmds) == 0 {
		return
	}
	if _, err := s.T.Run(ctx, cmds...); err != nil {
		s.last = nil // write them all again next time
		return
	}
	sidebar.RefreshAll(ctx, s.T)
}
