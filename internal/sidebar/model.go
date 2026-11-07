// Package sidebar is the sidebar pane on the left of each imux
// window (ARCHITECTURE D17, §9.3 "Sidebar"): the window's panes with their
// state, the agents the target pane can continue in, split buttons, the
// session picker, limits and keys. It draws with ANSI escapes and reads keys
// and SGR mouse events itself; actions run imux subcommands.
package sidebar

import (
	"context"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/gitinfo"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/textfmt"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// tmux options of the sidebar.
const (
	OptRole     = "@imux_role"        // pane: "sidebar" on sidebar panes
	RoleSidebar = "sidebar"           // value of OptRole
	OptPID      = "@imux_sidebar_pid" // pane: the sidebar process, signaled to redraw
	OptLast     = "@imux_last"        // window: the agent pane actions apply to
	OptState    = "@imux_state"       // pane: "working" or "idle", set by the watcher
)

// Pane states shown in the sidebar.
const (
	StateWorking = "working"
	StateIdle    = "idle"
	StateExited  = "exited"
	StateShell   = "shell"
)

// PaneRow is one agent (or shell) pane of a project.
type PaneRow struct {
	ID      string
	Agent   session.AgentID
	Name    string // the user's name for the pane (@imux_name), or ""
	Label   string // "Claude · opus"; "" for a shell
	State   string // working | idle | exited | shell
	Limited string // reset clock when the agent is limited, e.g. "15:00"
	Target  bool
}

// ProjectRow is one imux session: a project directory and its panes.
type ProjectRow struct {
	Session string
	Name    string // directory name
	Current bool
	Panes   []PaneRow
}

// ChangeRow is one `git status --short` entry.
type ChangeRow struct {
	Status string // "M", "A", "D", "R", "?" ...
	Path   string
}

// AgentRow is one agent the target pane can continue in.
type AgentRow struct {
	ID        session.AgentID
	Name      string // short name, "Claude"
	Vendor    string // "Anthropic"
	Key       string // menu mnemonic
	Installed bool
	Limited   string // reset clock, or ""
	Current   bool   // running in the target pane
}

// Model is everything the sidebar shows.
type Model struct {
	Project  string // directory name of the target pane
	CWD      string // its directory (diff popups run there)
	Branch   string // "" outside a git repository
	Changed  int    // files with uncommitted changes
	Git      bool
	Changes  []ChangeRow
	Projects []ProjectRow // the current project first
	Panes    []PaneRow    // the current window's panes (also inside Projects)
	Agents   []AgentRow
	Target   string // pane id actions apply to; "" when the window has none
	TargetW  int    // its size in cells (+ New agent picks the split direction)
	TargetH  int
}

// Window is what a refresh learned about the sidebar's own window.
type Window struct {
	ID      string
	Self    string // the sidebar's pane id
	Width   int    // the sidebar pane's width
	WinW    int    // the window's width
	Height  int
	Client  string // the client showing the session (most recently active)
	Session string
	Alone   bool // no other pane is left in the window
	Last    string
}

const paneFormat = "#{pane_id}\t#{@imux_role}\t#{@imux_agent}\t#{@imux_label}\t#{@imux_state}\t#{pane_active}\t#{pane_dead}\t#{@imux_cwd}\t#{pane_current_path}\t#{session_name}\t#{@imux_name}\t#{window_id}\t#{window_active}\t#{pane_width}\t#{pane_height}"

// OptName is the user's name for a pane.
const OptName = "@imux_name"

// Source gathers the model; it caches git information per directory.
type Source struct {
	T   *tmux.Client
	Cfg *config.Config
	Now func() time.Time
	// Resolve reports installed agents; nil = agent.ResolveAll.
	Resolve func(cfg *config.Config, st *state.State) map[session.AgentID]agent.Resolved
	// Git reads a directory's repository state; nil = gitinfo.Collect.
	Git func(ctx context.Context, dir string) gitinfo.Info

	mu  sync.Mutex
	git map[string]gitCache
}

type gitCache struct {
	at   time.Time
	info gitinfo.Info
}

const gitTTL = 20 * time.Second

func (s *Source) gitInfo(ctx context.Context, dir string) gitinfo.Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.git == nil {
		s.git = map[string]gitCache{}
	}
	if c, ok := s.git[dir]; ok && s.Now().Sub(c.at) < gitTTL {
		return c.info
	}
	collect := s.Git
	if collect == nil {
		collect = gitinfo.Collect
	}
	info := collect(ctx, dir)
	s.git[dir] = gitCache{at: s.Now(), info: info}
	return info
}

// Gather reads the window of pane self in one tmux exec, picks the target
// pane (the active one, else the last one focused, else the first) and
// records it in the window option @imux_last.
func (s *Source) Gather(ctx context.Context, self string) (Model, Window, error) {
	out, err := s.T.Run(ctx,
		tmux.DelimCmd("me"), tmux.Cmd{"display-message", "-p", "-t", self, "#{window_id}\t#{@imux_last}\t#{pane_width}\t#{pane_height}\t#{session_name}\t#{window_width}"},
		tmux.DelimCmd("clients"), tmux.Cmd{"list-clients", "-F", "#{client_name}\t#{client_session}\t#{client_activity}\t#{client_control_mode}"},
		tmux.DelimCmd("panes"), tmux.Cmd{"list-panes", "-a", "-F", paneFormat})
	if err != nil {
		return Model{}, Window{}, err
	}
	sec := tmux.Sections(out)
	win := Window{Self: self}
	if rows := tmux.Rows(sec["me"], 6); len(rows) > 0 {
		r := rows[0]
		win.ID, win.Last, win.Session = r[0], r[1], r[4]
		win.Width, _ = strconv.Atoi(r[2])
		win.Height, _ = strconv.Atoi(r[3])
		win.WinW, _ = strconv.Atoi(r[5])
	}
	var best int64 = -1
	for _, r := range tmux.Rows(sec["clients"], 4) {
		if r[1] != win.Session || r[3] == "1" {
			continue
		}
		act, _ := strconv.ParseInt(r[2], 10, 64)
		if act > best {
			best, win.Client = act, r[0]
		}
	}

	type raw struct {
		row     PaneRow
		active  bool
		cwd     string
		session string
		window  string
		w, h    int
	}
	var all []raw
	for _, r := range tmux.Rows(sec["panes"], 15) {
		if r[1] == RoleSidebar {
			continue
		}
		p := raw{
			row:    PaneRow{ID: r[0], Agent: session.AgentID(r[2]), Label: r[3], Name: r[10]},
			active: r[5] == "1" && r[12] == "1", cwd: r[7], session: r[9], window: r[11],
		}
		if p.cwd == "" {
			p.cwd = r[8]
		}
		p.w, _ = strconv.Atoi(r[13])
		p.h, _ = strconv.Atoi(r[14])
		switch {
		case r[6] == "1":
			p.row.State = StateExited
		case p.row.Agent == "":
			p.row.State = StateShell
		case r[4] == StateWorking:
			p.row.State = StateWorking
		default:
			p.row.State = StateIdle
		}
		all = append(all, p)
	}
	var panes []raw // this window's
	for _, p := range all {
		if p.window == win.ID {
			panes = append(panes, p)
		}
	}
	if len(panes) == 0 {
		win.Alone = true
		return Model{}, win, nil
	}
	target := -1
	for i, p := range panes {
		if p.active {
			target = i
		}
	}
	if target < 0 {
		for i, p := range panes {
			if p.row.ID == win.Last {
				target = i
			}
		}
	}
	if target < 0 {
		target = 0
	}
	panes[target].row.Target = true
	m := Model{Target: panes[target].row.ID, TargetW: panes[target].w, TargetH: panes[target].h}
	if m.Target != win.Last && win.ID != "" {
		_, _ = s.T.Run(ctx, tmux.Cmd{"set-option", "-w", "-t", win.ID, OptLast, m.Target})
		win.Last = m.Target
	}

	now := s.Now()
	st, _ := state.Read()
	if st == nil {
		st = &state.State{}
	}
	limited := func(id session.AgentID) string {
		l := st.ActiveLimit(string(id), now)
		if l == nil || !l.Blocking() {
			return ""
		}
		t := l.ResetAt
		if t.IsZero() {
			t = l.ExpiresAt
		}
		return textfmt.Clock(now, t)
	}
	fill := func(row PaneRow) PaneRow {
		if row.Agent != "" {
			row.Limited = limited(row.Agent)
			if row.Label == "" {
				row.Label = session.ShortName(row.Agent)
			}
		}
		return row
	}
	for _, p := range panes {
		m.Panes = append(m.Panes, fill(p.row))
	}
	// Projects: one per session, the current one first, then by name.
	bySession := map[string]*ProjectRow{}
	var order []string
	for _, p := range all {
		pr := bySession[p.session]
		if pr == nil {
			pr = &ProjectRow{Session: p.session, Name: filepath.Base(p.cwd), Current: p.session == win.Session}
			bySession[p.session] = pr
			order = append(order, p.session)
		}
		row := fill(p.row)
		row.Target = p.row.ID == m.Target
		pr.Panes = append(pr.Panes, row)
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := bySession[order[i]], bySession[order[j]]
		if a.Current != b.Current {
			return a.Current
		}
		return a.Name < b.Name
	})
	for _, name := range order {
		m.Projects = append(m.Projects, *bySession[name])
	}

	resolve := s.Resolve
	if resolve == nil {
		resolve = agent.ResolveAll
	}
	bins := resolve(s.Cfg, st)
	cur := panes[target].row.Agent
	if panes[target].row.State == StateExited {
		cur = ""
	}
	for _, a := range agent.Ordered(s.Cfg) {
		m.Agents = append(m.Agents, AgentRow{
			ID: a.ID, Name: a.Short, Vendor: a.Vendor, Key: a.Key, Installed: bins[a.ID].Installed,
			Limited: limited(a.ID), Current: a.ID == cur,
		})
	}

	cwd := panes[target].cwd
	m.Project, m.CWD = filepath.Base(cwd), cwd
	if cwd != "" {
		gi := s.gitInfo(ctx, cwd)
		if gi.State == gitinfo.OK {
			m.Git, m.Branch, m.Changed = true, gi.Branch, gi.StatusTotal
			m.Changes = ParseStatus(gi.Status)
		}
	}
	return m, win, nil
}

// RefreshAll signals every sidebar on the server to redraw (one tmux exec).
// Hooks, imux commands and the watcher call it after something changed.
func RefreshAll(ctx context.Context, t *tmux.Client) {
	out, err := t.Run(ctx, tmux.Cmd{"list-panes", "-a", "-F", "#{" + OptPID + "}"})
	if err != nil {
		return
	}
	var pids []int
	for _, line := range strings.Split(out, "\n") {
		if pid, err := strconv.Atoi(strings.TrimSpace(line)); err == nil && pid > 1 {
			pids = append(pids, pid)
		}
	}
	sort.Ints(pids)
	for _, pid := range pids {
		signalRedraw(pid)
	}
}

// ParseStatus turns `git status --short` lines into rows: "?? notes.md" is
// "?", "R  a -> b" keeps b.
func ParseStatus(lines []string) []ChangeRow {
	var out []ChangeRow
	for _, l := range lines {
		if len(l) < 4 {
			continue
		}
		st := strings.TrimSpace(l[:2])
		if st == "??" {
			st = "?"
		}
		if len(st) > 1 {
			st = st[:1]
		}
		path := strings.TrimSpace(l[3:])
		if _, after, ok := strings.Cut(path, " -> "); ok {
			path = after
		}
		out = append(out, ChangeRow{Status: st, Path: strings.Trim(path, `"`)})
	}
	return out
}
