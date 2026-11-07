package watch

import (
	"context"
	"sort"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// WorkingFor is how long after its last screen change a pane counts as
// working.
const WorkingFor = 3 * time.Second

// PaneStates writes each agent pane's state (@imux_state: working or idle)
// from the watcher's activity, only when it changes, and then makes the
// sidebars redraw (§9.3 "Sidebar").
type PaneStates struct {
	T    *tmux.Client
	last map[string]string
}

// Commands returns the set-option commands for states that changed.
func (s *PaneStates) Commands(at time.Time, changed map[string]time.Time) []tmux.Cmd {
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
	for _, p := range panes {
		seen[p] = true
		st := sidebar.StateIdle
		if at.Sub(changed[p]) < WorkingFor {
			st = sidebar.StateWorking
		}
		if s.last[p] == st {
			continue
		}
		s.last[p] = st
		cmds = append(cmds, tmux.Cmd{"set-option", "-p", "-q", "-t", p, sidebar.OptState, st})
	}
	for p := range s.last {
		if !seen[p] {
			delete(s.last, p) // the pane is gone
		}
	}
	return cmds
}

// Apply writes the changed states and signals the sidebars.
func (s *PaneStates) Apply(ctx context.Context, at time.Time, changed map[string]time.Time) {
	cmds := s.Commands(at, changed)
	if len(cmds) == 0 {
		return
	}
	if _, err := s.T.Run(ctx, cmds...); err != nil {
		s.last = nil // write them all again next time
		return
	}
	sidebar.RefreshAll(ctx, s.T)
}
