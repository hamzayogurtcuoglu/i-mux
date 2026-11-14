package workspace

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// ProjectInfo is what the project menus say about a session.
type ProjectInfo struct {
	Session string
	Dir     string // the project's folder
	Name    string // folder name
	Agents  int    // agent panes (sidebars and shells not counted)
	Windows int
}

// Project reads session s.
func (w *WS) Project(ctx context.Context, s string) (*ProjectInfo, error) {
	out, err := w.T.Run(ctx, tmux.Cmd{"list-panes", "-s", "-t", "=" + s, "-F", "#{session_path}\t#{@imux_role}\t#{@imux_agent}\t#{session_windows}"})
	if err != nil {
		return nil, err
	}
	pi := &ProjectInfo{Session: s}
	for _, r := range tmux.Rows(out, 4) {
		pi.Dir, pi.Name = r[0], filepath.Base(r[0])
		pi.Windows, _ = strconv.Atoi(r[3])
		if r[1] != sidebar.RoleSidebar && r[2] != "" {
			pi.Agents++
		}
	}
	if pi.Name == "" {
		return nil, errors.New(i18n.T("close.no_project", s))
	}
	return pi, nil
}

// ProjectMenu is the menu of a project in the sidebar: go to it (unless the
// client shows it) and close it.
func (w *WS) ProjectMenu(ctx context.Context, s, client, pane string) (*Menu, error) {
	pi, err := w.Project(ctx, s)
	if err != nil {
		return nil, err
	}
	m := &Menu{Title: pi.Name}
	if cur := w.clientSession(ctx, client); cur != s && client != "" {
		m.Items = append(m.Items, MenuItem{Name: i18n.T("project.go"), Key: "g", Cmd: "switch-client -c " + tmux.Quote(client) + " -t " + tmux.Quote("="+s)})
	}
	m.Items = append(m.Items, MenuItem{
		Name: i18n.T("project.close"), Key: "x",
		Cmd: w.ui(append([]string{"__menu", "--close-project", s}, uiTarget(pane, client)...)...),
	})
	// A worktree project: what its agent did, merged back, removed (D72).
	if wt, ok := WorktreeOf(ctx, pi.Dir); ok {
		m.Items = append(m.Items, w.worktreeItems(wt, s, client, pane)...)
	}
	return m, nil
}

// CloseProjectMenu asks before closing session s.
func (w *WS) CloseProjectMenu(ctx context.Context, s, client, pane string) (*Menu, error) {
	pi, err := w.Project(ctx, s)
	if err != nil {
		return nil, err
	}
	args := []string{"close", "--session", s}
	if client != "" {
		args = append(args, "--client", client)
	}
	return &Menu{
		Title: i18n.T("close.project_title", pi.Name, pi.Agents),
		Items: []MenuItem{
			{Name: i18n.T("close.yes_project"), Key: "y", Cmd: w.ui(args...)},
			{Name: i18n.T("click.cancel"), Key: "n"},
		},
	}, nil
}

// CloseTabMenu asks before closing the tab (window) of pane p; the last tab
// closes the project.
func (w *WS) CloseTabMenu(ctx context.Context, p *Pane, client string) (*Menu, error) {
	out, err := w.T.Run(ctx, tmux.Cmd{"list-panes", "-t", p.ID, "-F", "#{@imux_role}\t#{@imux_agent}\t#{session_windows}"})
	if err != nil {
		return nil, err
	}
	agents, windows := 0, 1
	for _, r := range tmux.Rows(out, 3) {
		windows, _ = strconv.Atoi(r[2])
		if r[0] != sidebar.RoleSidebar && r[1] != "" {
			agents++
		}
	}
	title := i18n.T("close.tab_title", agents)
	if windows <= 1 {
		title = i18n.T("close.last_tab_title", agents)
	}
	return &Menu{
		Title: title,
		Items: []MenuItem{
			{Name: i18n.T("close.yes_tab"), Key: "y", Cmd: w.ui(append([]string{"__close-tab"}, uiTarget(p.ID, client)...)...)},
			{Name: i18n.T("click.cancel"), Key: "n"},
		},
	}, nil
}

// clientSession is the session client shows ("" if unknown).
func (w *WS) clientSession(ctx context.Context, client string) string {
	if client == "" {
		return ""
	}
	out, err := w.T.Run(ctx, tmux.Cmd{"display-message", "-p", "-c", client, "#{client_session}"})
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// CloseProject ends session s. Clients showing it move to the most recently
// used other imux project first; without one they detach (§9.3).
func (w *WS) CloseProject(ctx context.Context, s string) error {
	out, err := w.T.Run(ctx,
		tmux.DelimCmd("sessions"), tmux.Cmd{"list-sessions", "-F", "#{session_name}\t#{session_activity}"},
		tmux.DelimCmd("clients"), tmux.Cmd{"list-clients", "-F", "#{client_name}\t#{client_session}"})
	if err != nil {
		return err
	}
	sec := tmux.Sections(out)
	type sess struct {
		name string
		act  int64
	}
	var others []sess
	found := false
	for _, r := range tmux.Rows(sec["sessions"], 2) {
		if r[0] == s {
			found = true
			continue
		}
		act, _ := strconv.ParseInt(r[1], 10, 64)
		others = append(others, sess{r[0], act})
	}
	if !found {
		return errors.New(i18n.T("close.no_project", s))
	}
	sort.Slice(others, func(i, j int) bool { return others[i].act > others[j].act })
	var cmds []tmux.Cmd
	if len(others) > 0 {
		for _, r := range tmux.Rows(sec["clients"], 2) {
			if r[1] == s {
				cmds = append(cmds, tmux.Cmd{"switch-client", "-c", r[0], "-t", "=" + others[0].name})
			}
		}
	}
	// Save it as it is, so that opening it again brings its tabs back.
	dir := ""
	if out, err := w.T.Run(ctx, tmux.Cmd{"display-message", "-p", "-t", "=" + s, "#{session_path}"}); err == nil {
		dir = strings.TrimSpace(out)
	}
	_ = w.SaveSnapshot(ctx)
	w.RememberNames(ctx, sources.New(w.Cfg), func(q *Pane) bool { return q.Session == s })
	cmds = append(cmds, tmux.Cmd{"kill-session", "-t", "=" + s})
	if _, err := w.T.Run(ctx, cmds...); err != nil {
		return err
	}
	if dir != "" {
		markClosed(dir)
	}
	w.RefreshSidebars(ctx)
	return nil
}

// CloseTab ends the tab (window) of pane p; the last tab closes the project.
func (w *WS) CloseTab(ctx context.Context, p *Pane) error {
	out, err := w.T.Run(ctx, tmux.Cmd{"display-message", "-p", "-t", p.ID, "#{session_name}\t#{session_windows}\t#{window_id}"})
	if err != nil {
		return err
	}
	rows := tmux.Rows(out, 3)
	if len(rows) == 0 {
		return errors.New("no window")
	}
	if n, _ := strconv.Atoi(rows[0][1]); n <= 1 {
		return w.CloseProject(ctx, rows[0][0])
	}
	win := rows[0][2]
	w.RememberNames(ctx, sources.New(w.Cfg), func(q *Pane) bool { return q.Window == win })
	if _, err := w.T.Run(ctx, tmux.Cmd{"kill-window", "-t", win}); err != nil {
		return err
	}
	w.RefreshSidebars(ctx)
	return nil
}

// agentInfo describes pane p for the agent menus: its shown name and how
// many agent panes its tab and project have.
func (w *WS) agentInfo(ctx context.Context, pane string) (name string, tabPanes, windows int, err error) {
	out, err := w.T.Run(ctx, tmux.Cmd{"list-panes", "-t", pane, "-F", "#{pane_id}\t#{@imux_role}\t#{@imux_agent}\t#{@imux_name}\t#{session_windows}"})
	if err != nil {
		return "", 0, 0, err
	}
	for _, r := range tmux.Rows(out, 5) {
		if r[1] == sidebar.RoleSidebar {
			continue
		}
		tabPanes++
		windows, _ = strconv.Atoi(r[4])
		if r[0] == pane {
			name = r[3]
			if name == "" && r[2] != "" {
				name = session.ShortName(session.AgentID(r[2]))
			}
			if name == "" {
				name = i18n.T("sidebar.shell")
			}
		}
	}
	return name, tabPanes, windows, nil
}

// AgentMenu is the menu of an agent pane in the sidebar: go to it, rename
// it, close it.
func (w *WS) AgentMenu(ctx context.Context, p *Pane, client string) (*Menu, error) {
	name, _, _, err := w.agentInfo(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	cur := ""
	if out, err := w.T.Run(ctx, tmux.Cmd{"show-options", "-pqv", "-t", p.ID, sidebar.OptName}); err == nil {
		cur = strings.TrimSpace(out)
	}
	rename := sidebar.RenamePrompt(w.Self, p.ID, client, cur, i18n.T("prompt.rename"))
	goTo := "select-pane -t " + p.ID
	if client != "" {
		goTo = "switch-client -c " + tmux.Quote(client) + " -t " + p.ID + " ; select-window -t " + p.ID + " ; " + goTo
	}
	return &Menu{
		Title: name,
		Items: []MenuItem{
			{Name: i18n.T("agent.go"), Key: "g", Cmd: goTo},
			{Name: i18n.T("agent.rename"), Key: "r", Cmd: rename},
			{Name: i18n.T("agent.close"), Key: "x", Cmd: w.ui(append([]string{"__menu", "--close-agent"}, uiTarget(p.ID, client)...)...)},
		},
	}, nil
}

// CloseAgentMenu asks before closing agent pane p.
func (w *WS) CloseAgentMenu(ctx context.Context, p *Pane, client string) (*Menu, error) {
	name, tabPanes, windows, err := w.agentInfo(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	title := i18n.T("close.agent_title", name)
	switch {
	case tabPanes <= 1 && windows <= 1:
		title = i18n.T("close.agent_last_project", name)
	case tabPanes <= 1:
		title = i18n.T("close.agent_last_tab", name)
	}
	return &Menu{
		Title: title,
		Items: []MenuItem{
			{Name: i18n.T("close.yes_agent"), Key: "y", Cmd: w.ui(append([]string{"__close-pane"}, uiTarget(p.ID, client)...)...)},
			{Name: i18n.T("click.cancel"), Key: "n"},
		},
	}, nil
}

// CloseAgent ends agent pane p. The last agent of a tab closes the tab (its
// sidebar leaves with it); the project's last one leaves the project open,
// saying "No agents" (§9.3 "A project without agents").
func (w *WS) CloseAgent(ctx context.Context, p *Pane) error {
	_, tabPanes, windows, err := w.agentInfo(ctx, p.ID)
	if err != nil {
		return err
	}
	// Its name goes on its conversation before the pane goes (names.go).
	w.RememberNames(ctx, sources.New(w.Cfg), func(q *Pane) bool { return q.ID == p.ID })
	switch {
	case tabPanes <= 1 && windows <= 1:
		return w.LeaveEmpty(ctx, p.ID)
	case tabPanes <= 1:
		return w.CloseTab(ctx, p)
	}
	if _, err := w.T.Run(ctx, tmux.Cmd{"kill-pane", "-t", p.ID}); err != nil {
		return err
	}
	w.RefreshSidebars(ctx)
	return nil
}

// LeaveEmpty replaces pane's agent with the "No agents" pane, in the
// project's folder.
func (w *WS) LeaveEmpty(ctx context.Context, pane string) error {
	p, err := w.Pane(ctx, pane)
	if err != nil {
		return err
	}
	cmds := append([]tmux.Cmd{{"respawn-pane", "-k", "-t", p.ID, "-c", p.CWD, w.Self, "__empty"}}, sidebar.EmptyOpts(p.ID, p.CWD)...)
	if _, err := w.T.Run(ctx, cmds...); err != nil {
		return err
	}
	w.RefreshSidebars(ctx)
	return nil
}

// IsEmpty reports whether p is the "No agents" pane.
func (p *Pane) IsEmpty() bool { return p.Role == sidebar.RoleEmpty }

// FillEmpty starts agent id (or, with id "", the user's shell) in the "No
// agents" pane p, which becomes an ordinary pane.
func (w *WS) FillEmpty(ctx context.Context, p *Pane, id session.AgentID) error {
	defer w.RefreshSidebars(ctx)
	if id != "" {
		return w.respawn(ctx, p, Launch{Agent: id, CWD: p.CWD}, nil, "")
	}
	_, err := w.T.Run(ctx,
		tmux.Cmd{"respawn-pane", "-k", "-t", p.ID, "-c", p.CWD, w.loginShell(ctx), "-l"},
		tmux.Cmd{"set-option", "-p", "-u", "-t", p.ID, sidebar.OptRole},
		tmux.Cmd{"set-option", "-p", "-u", "-t", p.ID, OptLabel})
	return err
}
