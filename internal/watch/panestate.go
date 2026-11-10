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
	// Notify gets the changes worth a notification (§9.3 "Notifications").
	Notify func(ctx context.Context, events []PaneEvent)
	since  map[string]time.Time // when each working pane started working
	events []PaneEvent
}

// PaneEvent is a change a notification is about.
type PaneEvent struct {
	Pane string
	Kind string // EventFinished or EventNeedsYou
}

// Event kinds.
const (
	EventFinished = "finished"
	EventNeedsYou = "needs"
)

// MinWork is how long an agent must have worked for its stop to be news.
const MinWork = 10 * time.Second

// Commands returns the set-option commands for states that changed. An idle
// screen that asks a question is blocked ("Needs you").
func (s *PaneStates) Commands(at time.Time, changed map[string]time.Time, asking map[string]bool) []tmux.Cmd {
	if s.last == nil {
		s.last = map[string]string{}
	}
	if s.since == nil {
		s.since = map[string]time.Time{}
	}
	s.events = s.events[:0]
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
		// prev "" is a pane the watcher just met (it started, or restarted
		// for a new config): no news then.
		switch {
		case st == sidebar.StateWorking:
			s.since[p] = changed[p]
		case prev == sidebar.StateWorking && st == sidebar.StateIdle:
			if start, ok := s.since[p]; ok && changed[p].Sub(start) >= MinWork {
				s.events = append(s.events, PaneEvent{Pane: p, Kind: EventFinished})
			}
			delete(s.since, p)
		case st == sidebar.StateBlocked && prev != "":
			s.events = append(s.events, PaneEvent{Pane: p, Kind: EventNeedsYou})
			delete(s.since, p)
		}
		set := func(k, v string) tmux.Cmd { return tmux.Cmd{"set-option", "-p", "-q", "-t", p, k, v} }
		cmds = append(cmds, set(sidebar.OptState, st), set(sidebar.OptStateAt, stamp))
		if prev == sidebar.StateWorking && st == sidebar.StateIdle {
			cmds = append(cmds, set(sidebar.OptDoneAt, stamp))
		}
	}
	for p := range s.last {
		if !seen[p] {
			delete(s.last, p) // the pane is gone
			delete(s.since, p)
		}
	}
	return cmds
}

// Load starts from the states tmux has (a watcher before this one wrote
// them): only real changes are written then, so an agent's "since" time
// survives a restart of the watcher.
func (s *PaneStates) Load(ctx context.Context) {
	out, err := s.T.Run(ctx, tmux.Cmd{"list-panes", "-a", "-F", "#{pane_id}\t#{" + sidebar.OptState + "}"})
	if err != nil {
		return
	}
	if s.last == nil {
		s.last = map[string]string{}
	}
	for _, r := range tmux.Rows(out, 2) {
		if r[1] != "" {
			s.last[r[0]] = r[1]
		}
	}
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
	if s.Notify != nil && len(s.events) > 0 {
		s.Notify(ctx, append([]PaneEvent{}, s.events...))
	}
}
