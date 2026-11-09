package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// PaneSnap is one agent pane of a saved tab (§9.3 "Recent and restore").
type PaneSnap struct {
	Agent session.AgentID `json:"agent,omitempty"` // "" for a shell
	Name  string          `json:"name,omitempty"`
	Run   *TrailEntry     `json:"run,omitempty"` // the pane's current run
}

// TabSnap is one saved tab (window).
type TabSnap struct {
	Name    string     `json:"name,omitempty"` // the user's tab name
	Layout  string     `json:"layout,omitempty"`
	Active  bool       `json:"active,omitempty"`
	Sidebar bool       `json:"sidebar,omitempty"`
	Panes   []PaneSnap `json:"panes"`
}

// ProjectSnap is a saved project: its folder and tabs.
type ProjectSnap struct {
	Dir    string    `json:"dir"`
	Tabs   []TabSnap `json:"tabs"`
	Server string    `json:"server,omitempty"` // the tmux server it was open in
	Open   bool      `json:"open,omitempty"`
	At     int64     `json:"at"` // unix seconds
}

// Agents counts the project's agent panes.
func (p ProjectSnap) Agents() int {
	n := 0
	for _, t := range p.Tabs {
		for _, q := range t.Panes {
			if q.Agent != "" {
				n++
			}
		}
	}
	return n
}

// Snapshots is ~/.imux/workspace.json.
type Snapshots struct {
	Projects []ProjectSnap `json:"projects"`
}

// MaxSnapshots bounds the projects kept.
const MaxSnapshots = 30

// Find returns the snapshot of dir.
func (s *Snapshots) Find(dir string) *ProjectSnap {
	for i := range s.Projects {
		if s.Projects[i].Dir == dir {
			return &s.Projects[i]
		}
	}
	return nil
}

// LoadSnapshots reads the snapshot file (empty when missing or broken).
func LoadSnapshots() *Snapshots {
	s := &Snapshots{}
	if b, err := os.ReadFile(paths.WorkspaceFile()); err == nil {
		_ = json.Unmarshal(b, s)
	}
	return s
}

// Save writes the snapshot file atomically (0600).
func (s *Snapshots) Save() error {
	b, err := json.MarshalIndent(s, "", " ")
	if err != nil {
		return err
	}
	if err := paths.EnsureDir(paths.Home()); err != nil {
		return err
	}
	tmp := paths.WorkspaceFile() + ".tmp"
	if err := paths.WriteFile(tmp, b); err != nil {
		return err
	}
	return os.Rename(tmp, paths.WorkspaceFile())
}

const snapFormat = "#{session_path}\t#{window_index}\t#{@imux_tab}\t#{window_layout}\t#{window_active}\t#{@imux_role}\t#{@imux_agent}\t#{@imux_name}\t#{pane_dead}\t#{@imux_trail}"

// ServerID identifies the running tmux server ("" when none runs).
func (w *WS) ServerID(ctx context.Context) string {
	out, err := w.T.Run(ctx, tmux.Cmd{"display-message", "-p", "#{pid}-#{start_time}"})
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// Capture reads every open project of the server.
func (w *WS) Capture(ctx context.Context) ([]ProjectSnap, error) {
	out, err := w.T.Run(ctx, tmux.Cmd{"list-panes", "-a", "-F", snapFormat})
	if err != nil {
		return nil, err
	}
	var projects []ProjectSnap
	byDir := map[string]int{}
	tabOf := map[string]int{} // dir + window index → tab
	for _, r := range tmux.Rows(out, 10) {
		dir := r[0]
		pi, ok := byDir[dir]
		if !ok {
			pi = len(projects)
			byDir[dir] = pi
			projects = append(projects, ProjectSnap{Dir: dir})
		}
		pr := &projects[pi]
		key := dir + "\x00" + r[1]
		ti, ok := tabOf[key]
		if !ok {
			ti = len(pr.Tabs)
			tabOf[key] = ti
			pr.Tabs = append(pr.Tabs, TabSnap{Name: r[2], Layout: r[3], Active: r[4] == "1"})
		}
		tab := &pr.Tabs[ti]
		if r[5] == sidebar.RoleSidebar {
			tab.Sidebar = true
			continue
		}
		if r[5] == sidebar.RoleEmpty {
			continue // "No agents": nothing to bring back
		}
		ps := PaneSnap{Name: r[7]}
		if r[8] != "1" && r[6] != "" {
			ps.Agent = session.AgentID(r[6])
			if l := ParseTrail(r[9]).Last(); l != nil && l.Agent == ps.Agent {
				e := *l
				ps.Run = &e
			}
		}
		tab.Panes = append(tab.Panes, ps)
	}
	return projects, nil
}

// Merge records the open projects of server at now: they replace their
// folders' snapshots; projects of this server that are gone were closed;
// those of an earlier server stay as they were (they died with it).
func (s *Snapshots) Merge(open []ProjectSnap, server string, now time.Time) {
	seen := map[string]bool{}
	for _, p := range open {
		p.Open, p.Server, p.At = true, server, now.Unix()
		seen[p.Dir] = true
		if old := s.Find(p.Dir); old != nil {
			*old = p
		} else {
			s.Projects = append(s.Projects, p)
		}
	}
	for i := range s.Projects {
		p := &s.Projects[i]
		if !seen[p.Dir] && p.Open && p.Server == server {
			p.Open = false
		}
	}
	sort.SliceStable(s.Projects, func(i, j int) bool { return s.Projects[i].At > s.Projects[j].At })
	if len(s.Projects) > MaxSnapshots {
		s.Projects = s.Projects[:MaxSnapshots]
	}
}

// SaveSnapshot captures the open projects into the snapshot file.
func (w *WS) SaveSnapshot(ctx context.Context) error {
	server := w.ServerID(ctx)
	if server == "" {
		return nil
	}
	open, err := w.Capture(ctx)
	if err != nil {
		return err
	}
	s := LoadSnapshots()
	s.Merge(open, server, w.Now())
	return s.Save()
}

// markClosed records that the project of dir was closed on purpose.
func markClosed(dir string) {
	s := LoadSnapshots()
	if p := s.Find(dir); p != nil && p.Open {
		p.Open = false
		_ = s.Save()
	}
}

// Interrupted lists the projects that were open in an earlier tmux server
// (it went away, e.g. with a reboot) and are not open now.
func (s *Snapshots) Interrupted(server string, open map[string]bool) []ProjectSnap {
	var out []ProjectSnap
	for _, p := range s.Projects {
		if p.Open && p.Server != server && !open[p.Dir] && p.Agents() > 0 {
			out = append(out, p)
		}
	}
	return out
}

// Restore recreates a saved project: a session in its folder, a window per
// tab, a pane per agent (each resuming its conversation when it can), the
// names, sidebars and layouts (§9.3 "Recent and restore").
func (w *WS) Restore(ctx context.Context, f *sources.Finder, snap *ProjectSnap) (*StartResult, error) {
	name := SessionName(snap.Dir)
	if ok, err := w.HasSession(ctx, name); err != nil {
		return nil, err
	} else if ok {
		return &StartResult{Session: name}, nil
	}
	if fi, err := os.Stat(snap.Dir); err != nil || !fi.IsDir() {
		return nil, errors.New("folder gone: " + paths.Shorten(snap.Dir))
	}
	res := &StartResult{Session: name, Created: true}
	activeWin := ""
	for _, tab := range snap.Tabs {
		var panes []PaneSnap
		for _, p := range tab.Panes {
			if p.Agent != "" && w.Installed(p.Agent) {
				panes = append(panes, p)
			}
		}
		if len(panes) == 0 {
			continue
		}
		first := ""
		for i, p := range panes {
			l := Launch{Agent: p.Agent, CWD: snap.Dir, ResumeID: w.resumeID(ctx, f, p, snap.Dir)}
			var create func(cmd []string) tmux.Cmd
			switch {
			case i > 0:
				create = func(cmd []string) tmux.Cmd {
					// No -d: the options that follow in the batch go to the
					// new pane, which must be the current one.
					return append(tmux.Cmd{"split-window", "-h", "-t", first, "-c", snap.Dir, "-P", "-F", "#{pane_id}"}, cmd...)
				}
			case res.Pane == "":
				create = func(cmd []string) tmux.Cmd {
					return append(tmux.Cmd{"new-session", "-d", "-s", name, "-c", snap.Dir, "-P", "-F", "#{pane_id}"}, cmd...)
				}
			default:
				create = func(cmd []string) tmux.Cmd {
					return append(tmux.Cmd{"new-window", "-t", "=" + name + ":", "-c", snap.Dir, "-P", "-F", "#{pane_id}"}, cmd...)
				}
			}
			pane, err := w.launchNew(ctx, l, create)
			if err != nil && l.ResumeID != "" {
				l.ResumeID = "" // start it fresh rather than lose the tab
				pane, err = w.launchNew(ctx, l, create)
			}
			if err != nil {
				if res.Pane == "" {
					return nil, err
				}
				continue
			}
			if i == 0 {
				first = pane
				if res.Pane == "" {
					res.Pane = pane
				}
			}
			if p.Name != "" {
				_, _ = w.T.Run(ctx, tmux.Cmd{"set-option", "-p", "-t", pane, sidebar.OptName, p.Name})
			}
		}
		if first == "" {
			continue
		}
		if tab.Name != "" {
			_, _ = w.T.Run(ctx, tmux.Cmd{"set-option", "-w", "-t", first, "@imux_tab", tab.Name})
		}
		if tab.Sidebar {
			w.maybeSidebar(ctx, first)
		}
		if tab.Layout != "" && len(panes) == len(tab.Panes) {
			_, _ = w.T.Run(ctx, tmux.Cmd{"select-layout", "-t", first, tab.Layout})
		}
		if tab.Active {
			activeWin = first
		}
	}
	if res.Pane == "" {
		return nil, errors.New("nothing to restore")
	}
	if activeWin != "" {
		_, _ = w.T.Run(ctx, tmux.Cmd{"select-window", "-t", activeWin})
	}
	EnsureWatcher(w)
	return res, nil
}

// resumeID is the session a saved pane's agent resumes ("" = fresh): only
// the recorded session, or the one its run wrote (found by RefFor), never a
// look-alike.
func (w *WS) resumeID(ctx context.Context, f *sources.Finder, p PaneSnap, dir string) string {
	ac := w.Cfg.Agent(string(p.Agent))
	if p.Run == nil || !ac.Resume || len(ac.ResumeArgs) == 0 || f == nil {
		return ""
	}
	ref, err := w.RefFor(ctx, f, *p.Run, dir)
	if err != nil || ref.ID == "" {
		return ""
	}
	if p.Run.SessionID != "" && ref.ID != p.Run.SessionID && (p.Run.File == "" || ref.Path != p.Run.File) {
		return ""
	}
	if _, err := os.Stat(ref.Path); err != nil {
		return ""
	}
	return ref.ID
}

// Summary is "3 tabs · 4 agents" for the picker.
func (p ProjectSnap) Summary(tabs, agents func(n int) string) string {
	n := 0
	for _, t := range p.Tabs {
		for _, q := range t.Panes {
			if q.Agent != "" {
				n++
				break
			}
		}
	}
	return tabs(n) + " · " + agents(p.Agents())
}

// AbsDir cleans a folder path for comparisons.
func AbsDir(dir string) string {
	if a, err := filepath.Abs(dir); err == nil {
		return filepath.Clean(a)
	}
	return dir
}
