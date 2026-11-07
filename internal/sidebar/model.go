// Package sidebar is the sidebar pane on the left of each imux
// window (ARCHITECTURE D17, §9.3 "Sidebar"): the window's panes with their
// state, the agents the target pane can continue in, split buttons, the
// session picker, limits and keys. It draws with ANSI escapes and reads keys
// and SGR mouse events itself; actions run imux subcommands.
package sidebar

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

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
	OptState    = "@imux_state"       // pane: "working", "idle" or "blocked", set by the watcher
	OptStateAt  = "@imux_state_at"    // pane: when the state last changed (unix seconds)
	OptDoneAt   = "@imux_done_at"     // pane: when its work last ended (unix seconds)
	OptSeen     = "@imux_seen"        // pane: when the user last looked at it (unix seconds)
	OptTabAuto  = "@imux_tab_auto"    // window: the tab's name from its agent (tab bar)
	OptTabState = "@imux_tab_state"   // window: the tab's state glyph with its style (tab bar)
)

// TabName is a tab's name from its panes: the target pane's name (# escaped
// for the status line).
func TabName(panes []PaneRow) string {
	for _, p := range panes {
		if p.Target {
			return strings.ReplaceAll(p.Display("shell"), "#", "##")
		}
	}
	return ""
}

// TabState is the most urgent state of a tab's panes as a styled glyph for
// the tab bar ("" when all are idle).
func TabState(panes []PaneRow) string {
	st, ok := rollup(panes)
	if !ok {
		return ""
	}
	switch {
	case st.State == StateBlocked:
		return "#[fg=colour214]!"
	case st.Limited != "":
		return "#[fg=colour203]◆"
	case st.State == StateDone:
		return "#[fg=colour71]●"
	}
	return "#[fg=colour75]●"
}

// Pane states shown in the sidebar.
const (
	StateWorking = "working"
	StateIdle    = "idle"
	StateBlocked = "blocked" // the agent asks the user something: "Needs you"
	StateDone    = "done"    // work ended while the user looked elsewhere
	StateExited  = "exited"
	StateShell   = "shell"
)

// PaneRow is one agent (or shell) pane of a project.
type PaneRow struct {
	ID      string
	Agent   session.AgentID
	Name    string    // the user's name for the pane (@imux_name), or ""
	Title   string    // the terminal title the agent set, cleaned ("" if none)
	Label   string    // "Claude · opus"; "" for a shell
	State   string    // working | blocked | done | idle | exited | shell
	Since   time.Time // when the state began (zero if unknown)
	Limited string    // reset clock when the agent is limited, e.g. "15:00"
	Target  bool
}

// Display is the row's name: the user's name, else the agent's title, else
// the agent (or "Shell").
func (p PaneRow) Display(shell string) string {
	switch {
	case p.Name != "":
		return p.Name
	case p.Title != "":
		return p.Title
	case p.Agent == "":
		return shell
	}
	return session.ShortName(p.Agent)
}

// ProjectRow is one imux session: a project directory and its panes.
type ProjectRow struct {
	Session string
	Name    string // directory name
	Branch  string // "" outside git
	Dirty   bool   // uncommitted changes
	Current bool
	Panes   []PaneRow
}

// ChangeRow is one `git status --short` entry.
type ChangeRow struct {
	Status  string // "M", "A", "D", "R", "?" ...
	Path    string
	Added   int // lines (git diff --numstat HEAD); 0 when unknown
	Removed int
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
	Now      time.Time       // for the times shown
	Tick     int             // spinner frame
	Fold     map[string]bool // folded sections (@imux_fold)
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

const paneFormat = "#{pane_id}\t#{@imux_role}\t#{@imux_agent}\t#{@imux_label}\t#{@imux_state}\t#{pane_active}\t#{pane_dead}\t#{@imux_cwd}\t#{pane_current_path}\t#{session_name}\t#{@imux_name}\t#{window_id}\t#{window_active}\t#{pane_width}\t#{pane_height}\t#{@imux_state_at}\t#{@imux_done_at}\t#{@imux_seen}\t#{@imux_start}\t#{session_attached}\t#{pane_title}"

const paneFields = 21

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
	// Numstat returns `git diff --numstat HEAD` of a directory; nil runs git.
	Numstat func(ctx context.Context, dir string) string
	// Host is this machine's name (a terminal title equal to it is not a
	// name); nil = os.Hostname.
	Host func() string

	mu  sync.Mutex
	git map[string]gitCache
}

type gitCache struct {
	at    time.Time
	info  gitinfo.Info
	num   map[string][2]int
	numAt time.Time
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

func (s *Source) host() string {
	if s.Host != nil {
		return s.Host()
	}
	h, _ := os.Hostname()
	return h
}

// unix parses a unix-seconds option ("" or junk is zero).
func unix(v string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	return n
}

// CleanTitle turns a terminal title into a pane name: spinner and status
// glyphs in front are dropped; the host name, a program or agent name and
// paths are not names ("").
func CleanTitle(title, host string, agentID session.AgentID) string {
	if t := strings.TrimSpace(title); strings.HasPrefix(t, "/") || strings.HasPrefix(t, "~") {
		return ""
	}
	t := strings.TrimLeftFunc(title, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	t = strings.TrimSpace(t)
	if t == "" {
		return ""
	}
	low := strings.ToLower(t)
	short := strings.ToLower(strings.SplitN(host, ".", 2)[0])
	if low == strings.ToLower(host) || low == short || strings.HasPrefix(low, short+".") {
		return ""
	}
	for _, n := range []string{"node", "bash", "zsh", "sh", "fish", "tmux", "imux", "i-mux"} {
		if low == n {
			return ""
		}
	}
	if agentID != "" {
		a := agent.MustGet(agentID)
		for _, n := range []string{a.Bin, a.Name, a.Short, string(agentID)} {
			if low == strings.ToLower(n) {
				return ""
			}
		}
	}
	return t
}

// Gather reads every pane in one tmux exec, picks the window's target pane
// (the active one, else the last one focused, else the first) and records
// it in the window option @imux_last. The pane the user is looking at (the
// active pane of an attached session's current window) is marked seen
// (@imux_seen), which turns its "Done" into idle.
func (s *Source) Gather(ctx context.Context, self string) (Model, Window, error) {
	out, err := s.T.Run(ctx,
		tmux.DelimCmd("me"), tmux.Cmd{"display-message", "-p", "-t", self, "#{window_id}\t#{@imux_last}\t#{pane_width}\t#{pane_height}\t#{session_name}\t#{window_width}\t#{@imux_tab_auto}\t#{@imux_tab_state}\t#{@imux_fold}"},
		tmux.DelimCmd("clients"), tmux.Cmd{"list-clients", "-F", "#{client_name}\t#{client_session}\t#{client_activity}\t#{client_control_mode}"},
		tmux.DelimCmd("panes"), tmux.Cmd{"list-panes", "-a", "-F", paneFormat})
	if err != nil {
		return Model{}, Window{}, err
	}
	sec := tmux.Sections(out)
	win := Window{Self: self}
	var tabAuto, tabState string
	fold := map[string]bool{}
	if rows := tmux.Rows(sec["me"], 9); len(rows) > 0 {
		r := rows[0]
		tabAuto, tabState = r[6], r[7]
		for _, f := range strings.Split(r[8], ",") {
			if f = strings.TrimSpace(f); f != "" {
				fold[f] = true
			}
		}
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

	now := s.Now()
	host := s.host()
	type raw struct {
		row     PaneRow
		active  bool // active pane of its window's active window
		looked  bool // ... of an attached session: the user sees it
		cwd     string
		session string
		window  string
		w, h    int
	}
	var all []raw
	var seen []tmux.Cmd
	for _, r := range tmux.Rows(sec["panes"], paneFields) {
		if r[1] == RoleSidebar {
			continue
		}
		p := raw{
			row:    PaneRow{ID: r[0], Agent: session.AgentID(r[2]), Label: r[3], Name: r[10]},
			active: r[5] == "1" && r[12] == "1", cwd: r[7], session: r[9], window: r[11],
		}
		p.looked = p.active && r[19] != "" && r[19] != "0"
		if p.cwd == "" {
			p.cwd = r[8]
		}
		p.w, _ = strconv.Atoi(r[13])
		p.h, _ = strconv.Atoi(r[14])
		stateAt, doneAt, seenAt, start := unix(r[15]), unix(r[16]), unix(r[17]), unix(r[18])
		if p.row.Agent != "" {
			p.row.Title = CleanTitle(r[20], host, p.row.Agent)
		}
		switch {
		case stateAt > 0:
			p.row.Since = time.Unix(stateAt, 0)
		case start > 0:
			p.row.Since = time.UnixMilli(start)
		}
		switch {
		case r[6] == "1":
			p.row.State = StateExited
		case p.row.Agent == "":
			p.row.State = StateShell
		case r[4] == StateWorking:
			p.row.State = StateWorking
		case r[4] == StateBlocked:
			p.row.State = StateBlocked
		case doneAt > seenAt && !p.looked:
			p.row.State = StateDone
		default:
			p.row.State = StateIdle
		}
		if p.looked && doneAt > seenAt {
			seen = append(seen, tmux.Cmd{"set-option", "-p", "-q", "-t", p.row.ID, OptSeen, strconv.FormatInt(now.Unix(), 10)})
		}
		all = append(all, p)
	}
	if len(seen) > 0 {
		_, _ = s.T.Run(ctx, seen...)
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
	m := Model{Target: panes[target].row.ID, TargetW: panes[target].w, TargetH: panes[target].h, Now: now, Fold: fold}
	if m.Target != win.Last && win.ID != "" {
		_, _ = s.T.Run(ctx, tmux.Cmd{"set-option", "-w", "-t", win.ID, OptLast, m.Target})
		win.Last = m.Target
	}

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
	// The tab bar shows this window's name and state (§9.3 "Tabs").
	if win.ID != "" {
		var set []tmux.Cmd
		if name := TabName(m.Panes); name != tabAuto {
			set = append(set, tmux.Cmd{"set-option", "-w", "-q", "-t", win.ID, OptTabAuto, name})
		}
		if st := TabState(m.Panes); st != tabState {
			set = append(set, tmux.Cmd{"set-option", "-w", "-q", "-t", win.ID, OptTabState, st})
		}
		if len(set) > 0 {
			_, _ = s.T.Run(ctx, set...)
		}
	}
	// Projects: one per session, the current one first, then by name.
	bySession := map[string]*ProjectRow{}
	cwdOf := map[string]string{}
	var order []string
	for _, p := range all {
		pr := bySession[p.session]
		if pr == nil {
			pr = &ProjectRow{Session: p.session, Name: filepath.Base(p.cwd), Current: p.session == win.Session}
			bySession[p.session] = pr
			cwdOf[p.session] = p.cwd
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
		pr := bySession[name]
		if dir := cwdOf[name]; dir != "" && !pr.Current {
			if gi := s.gitInfo(ctx, dir); gi.State == gitinfo.OK {
				pr.Branch, pr.Dirty = gi.Branch, gi.StatusTotal > 0
			}
		}
		m.Projects = append(m.Projects, *pr)
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
			if len(m.Changes) > 0 {
				ns := s.numstat(ctx, cwd)
				for i := range m.Changes {
					if n, ok := ns[m.Changes[i].Path]; ok {
						m.Changes[i].Added, m.Changes[i].Removed = n[0], n[1]
					}
				}
			}
			for i := range m.Projects {
				if m.Projects[i].Current {
					m.Projects[i].Branch, m.Projects[i].Dirty = gi.Branch, gi.StatusTotal > 0
				}
			}
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

// numstat returns the added and removed lines per changed file of dir
// (git diff --numstat HEAD), cached like the status.
func (s *Source) numstat(ctx context.Context, dir string) map[string][2]int {
	s.mu.Lock()
	c := s.git[dir]
	if c.num != nil && s.Now().Sub(c.numAt) < gitTTL {
		s.mu.Unlock()
		return c.num
	}
	s.mu.Unlock()
	run := s.Numstat
	if run == nil {
		run = gitNumstat
	}
	num := ParseNumstat(run(ctx, dir))
	s.mu.Lock()
	c = s.git[dir]
	c.num, c.numAt = num, s.Now()
	if s.git == nil {
		s.git = map[string]gitCache{}
	}
	s.git[dir] = c
	s.mu.Unlock()
	return num
}

// gitNumstat runs git diff --numstat HEAD in dir (2 s at most).
func gitNumstat(ctx context.Context, dir string) string {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "diff", "--numstat", "HEAD").Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// ParseNumstat reads `git diff --numstat` output: "12\t3\tpath"; binary
// files ("-\t-") count 0; a rename "a => b" (or "{a => b}/c") keeps the new path.
func ParseNumstat(out string) map[string][2]int {
	m := map[string][2]int{}
	for _, l := range strings.Split(out, "\n") {
		f := strings.SplitN(l, "\t", 3)
		if len(f) != 3 {
			continue
		}
		a, _ := strconv.Atoi(f[0])
		r, _ := strconv.Atoi(f[1])
		path := f[2]
		if i := strings.Index(path, "{"); i >= 0 {
			if j := strings.Index(path[i:], "}"); j > 0 {
				inner := path[i+1 : i+j]
				if _, after, ok := strings.Cut(inner, " => "); ok {
					path = strings.ReplaceAll(path[:i]+after+path[i+j+1:], "//", "/")
				}
			}
		} else if _, after, ok := strings.Cut(path, " => "); ok {
			path = after
		}
		m[path] = [2]int{a, r}
	}
	return m
}
