package watch

import (
	"sort"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/textfmt"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

// Markers shown in the status bar, the agent bar and pane borders.
const (
	MarkLimited = "⛔"
	MarkWarn    = "◐"
	MarkOK      = "●"
)

// StatusKeeper keeps @imux_status, @imux_lim_<agent> and @imux_flag in
// line with state.json, setting an option only when its value changes.
type StatusKeeper struct {
	W    *workspace.WS
	last map[string]string // option target key → value set
}

// Commands returns the set-option commands needed this tick (nil when
// nothing changed). Expired limits are dropped from state.json first.
func (s *StatusKeeper) Commands(panes []PaneRow) []tmux.Cmd {
	w := s.W
	w.Refresh()
	resolve(w)
	now := w.Now()
	st := w.State()
	if hasExpired(st, now) {
		_ = state.Update(func(x *state.State) error { x.DropExpired(now); return nil })
		w.Refresh()
		resolve(w)
		st = w.State()
	}
	want := map[string]string{}
	var parts []string
	for _, a := range agent.Ordered(w.Cfg) {
		l := st.ActiveRecord(string(a.ID), now)
		mark := ""
		switch {
		case l == nil:
		case l.Blocking():
			mark = MarkLimited + clockOf(now, l)
		default:
			mark = MarkWarn
		}
		want["g:@imux_lim_"+string(a.ID)] = mark
		if !w.Installed(a.ID) {
			continue
		}
		if mark == "" {
			mark = MarkOK
		}
		parts = append(parts, a.Short+" "+mark)
	}
	want["g:@imux_status"] = strings.Join(parts, " · ")
	for _, p := range panes {
		if p.Agent == "" {
			continue
		}
		flag := ""
		if l := st.ActiveLimit(string(p.Agent), now); l != nil && !p.Dead {
			flag = MarkLimited + " " + clockOf(now, l)
		}
		want["p:"+p.ID] = flag
	}
	if s.last == nil {
		s.last = map[string]string{}
	}
	keys := make([]string, 0, len(want))
	for k := range want {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var cmds []tmux.Cmd
	for _, k := range keys {
		v := want[k]
		if old, ok := s.last[k]; ok && old == v {
			continue
		}
		if pane, ok := strings.CutPrefix(k, "p:"); ok {
			if v == "" {
				cmds = append(cmds, tmux.Cmd{"set-option", "-p", "-u", "-t", pane, workspace.OptFlag})
			} else {
				cmds = append(cmds, tmux.Cmd{"set-option", "-p", "-t", pane, workspace.OptFlag, v})
			}
		} else {
			cmds = append(cmds, tmux.Cmd{"set-option", "-g", strings.TrimPrefix(k, "g:"), v})
		}
		s.last[k] = v
	}
	// Forget panes that are gone.
	for k := range s.last {
		if _, ok := want[k]; !ok {
			delete(s.last, k)
		}
	}
	return cmds
}

// Forget makes the next Commands set every option again (after a failed
// exec, or when the server restarted).
func (s *StatusKeeper) Forget() { s.last = nil }

func hasExpired(st *state.State, now time.Time) bool {
	for _, l := range st.Limits {
		if !l.Active(now) {
			return true
		}
	}
	return false
}

func clockOf(now time.Time, l *state.Limit) string {
	t := l.ResetAt
	if t.IsZero() {
		t = l.ExpiresAt
	}
	return textfmt.Clock(now, t)
}
