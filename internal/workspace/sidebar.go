package workspace

import (
	"context"
	"strconv"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/gitinfo"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// IsSidebar reports whether p is a sidebar pane.
func (p *Pane) IsSidebar() bool { return p.Role == sidebar.RoleSidebar }

// Target returns the pane commands given p act on: p itself, or for a
// sidebar the agent pane last focused in its window (@imux_last), else the
// window's first other pane.
func (w *WS) Target(ctx context.Context, p *Pane) (*Pane, error) {
	if !p.IsSidebar() {
		return p, nil
	}
	out, err := w.T.Run(ctx,
		tmux.DelimCmd("panes"), tmux.Cmd{"list-panes", "-t", p.ID, "-F", paneFormat},
		tmux.DelimCmd("last"), tmux.Cmd{"display-message", "-p", "-t", p.ID, "#{" + sidebar.OptLast + "}"})
	if err != nil {
		return nil, err
	}
	sec := tmux.Sections(out)
	last := strings.TrimSpace(sec["last"])
	var first *Pane
	for _, r := range tmux.Rows(sec["panes"], paneFields) {
		q := parsePane(r)
		if q.IsSidebar() {
			continue
		}
		if q.ID == last {
			return q, nil
		}
		if first == nil {
			first = q
		}
	}
	if first == nil {
		return p, nil
	}
	return first, nil
}

// PaneTarget reads pane id and resolves it with Target.
func (w *WS) PaneTarget(ctx context.Context, id string) (*Pane, error) {
	p, err := w.Pane(ctx, id)
	if err != nil {
		return nil, err
	}
	return w.Target(ctx, p)
}

// sidebarCmd creates a sidebar left of the window of pane, full height,
// without taking the focus.
func (w *WS) sidebarCmd(pane string) tmux.Cmd {
	width := w.Cfg.Tmux.SidebarWidth
	if width <= 0 {
		width = 30
	}
	return tmux.Cmd{
		"split-window", "-h", "-b", "-f", "-d", "-l", strconv.Itoa(width), "-t", pane,
		"-P", "-F", "#{pane_id}", w.Self, "__sidebar",
	}
}

// AddSidebar opens a sidebar in pane's window (when the config wants one).
func (w *WS) AddSidebar(ctx context.Context, pane string) (string, error) {
	out, err := w.T.Run(ctx, w.sidebarCmd(pane))
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(strings.SplitN(out, "\n", 2)[0])
	_, err = w.T.Run(ctx,
		tmux.Cmd{"set-option", "-p", "-t", id, sidebar.OptRole, sidebar.RoleSidebar},
		tmux.Cmd{"set-option", "-p", "-t", id, "remain-on-exit", "off"},
		tmux.Cmd{"set-option", "-p", "-t", id, "@imux_label", "i-mux"},
		tmux.Cmd{"set-option", "-w", "-t", pane, sidebar.OptLast, pane})
	return id, err
}

// maybeSidebar adds a sidebar to a new window when configured; a failure
// only costs the sidebar.
func (w *WS) maybeSidebar(ctx context.Context, pane string) {
	if w.Cfg.Tmux.Sidebar {
		_, _ = w.AddSidebar(ctx, pane)
	}
}

// windowPanes lists the panes of pane's window.
func (w *WS) windowPanes(ctx context.Context, pane string) ([]*Pane, error) {
	out, err := w.T.Run(ctx, tmux.Cmd{"list-panes", "-t", pane, "-F", paneFormat})
	if err != nil {
		return nil, err
	}
	var ps []*Pane
	for _, r := range tmux.Rows(out, paneFields) {
		ps = append(ps, parsePane(r))
	}
	return ps, nil
}

// ToggleSidebar shows (on), hides (off) or flips (toggle) the sidebar of
// pane's window. It reports whether a sidebar is shown afterwards.
func (w *WS) ToggleSidebar(ctx context.Context, pane, mode string) (bool, error) {
	ps, err := w.windowPanes(ctx, pane)
	if err != nil {
		return false, err
	}
	var bars []string
	target := ""
	for _, p := range ps {
		if p.IsSidebar() {
			bars = append(bars, p.ID)
		} else if target == "" || p.ID == pane {
			target = p.ID
		}
	}
	show := len(bars) == 0
	switch mode {
	case "on":
		show = true
	case "off":
		show = false
	}
	if !show {
		// Remembered, so the watcher does not bring it back.
		_, _ = w.T.Run(ctx, tmux.Cmd{"set-option", "-w", "-t", pane, OptSidebarOff, "1"})
		for _, id := range bars {
			_, _ = w.T.Run(ctx, tmux.Cmd{"kill-pane", "-t", id})
		}
		return false, nil
	}
	_, _ = w.T.Run(ctx, tmux.Cmd{"set-option", "-w", "-u", "-t", pane, OptSidebarOff})
	if len(bars) > 0 || target == "" {
		return len(bars) > 0, nil
	}
	_, err = w.AddSidebar(ctx, target)
	return err == nil, err
}

// RefreshSidebars makes every sidebar redraw.
func (w *WS) RefreshSidebars(ctx context.Context) { sidebar.RefreshAll(ctx, w.T) }

// SplitMenu asks which agent the new pane next to p starts with.
func (w *WS) SplitMenu(p *Pane, client string, horizontal bool) *Menu {
	dir, title := "-v", i18n.T("split.down")
	if horizontal {
		dir, title = "-h", i18n.T("split.right")
	}
	m := &Menu{Title: title}
	for _, a := range agent.Ordered(w.Cfg) {
		it := MenuItem{Name: a.Short, Key: a.Key, Cmd: w.ui(append([]string{"split", string(a.ID), dir}, uiTarget(p.ID, client)...)...)}
		if !w.Installed(a.ID) {
			it.Name += " " + i18n.T("menu.not_installed")
			it.Disabled, it.Key = true, ""
		} else if until := w.LimitedUntil(a.ID); until != "" {
			it.Name += " " + i18n.T("sidebar.limited", until)
		}
		m.Items = append(m.Items, it)
	}
	m.Items = append(m.Items, Separator(), MenuItem{
		Name: i18n.T("menu.terminal"), Key: "t",
		Cmd: w.ui(append([]string{"split", ShellAgent, dir}, uiTarget(p.ID, client)...)...),
	})
	return m
}

// NewAgentMenu asks which agent a new pane starts with (the sidebar's "+"
// of Agents); PlaceMenu then asks where it opens. In the project's "No
// agents" pane the agent starts in its place.
func (w *WS) NewAgentMenu(p *Pane, client string) *Menu {
	if p.IsEmpty() {
		return w.SplitMenu(p, client, true)
	}
	m := &Menu{Title: i18n.T("new_agent.title")}
	for _, a := range agent.Ordered(w.Cfg) {
		it := MenuItem{Name: a.Short, Key: a.Key, Cmd: w.ui(append([]string{"__menu", "--place", string(a.ID)}, uiTarget(p.ID, client)...)...)}
		if !w.Installed(a.ID) {
			it.Name += " " + i18n.T("menu.not_installed")
			it.Disabled, it.Key = true, ""
		} else if until := w.LimitedUntil(a.ID); until != "" {
			it.Name += " " + i18n.T("sidebar.limited", until)
		}
		m.Items = append(m.Items, it)
	}
	m.Items = append(m.Items, Separator(), MenuItem{
		Name: i18n.T("menu.terminal"), Key: "t",
		Cmd: w.ui(append([]string{"__menu", "--place", ShellAgent}, uiTarget(p.ID, client)...)...),
	})
	if w.Cfg.Web.Enabled {
		m.Items = append(m.Items, MenuItem{Name: i18n.T("menu.web_page"), Key: "w", Cmd: w.WebPrompt(p.ID, client)})
	}
	return m
}

// PlaceMenu asks where agent id (or ShellAgent) opens: in a new pane right
// of, left of, below or above p, or in a new tab.
func (w *WS) PlaceMenu(p *Pane, client, id string) *Menu {
	name := i18n.T("menu.terminal")
	if a, ok := agent.Get(session.AgentID(id)); ok {
		name = a.Short
	}
	split := func(dir string, before bool) string {
		args := []string{"split", id, dir}
		if before {
			args = append(args, "--before")
		}
		return w.ui(append(args, uiTarget(p.ID, client)...)...)
	}
	return &Menu{Title: i18n.T("place.title", name), Items: []MenuItem{
		{Name: i18n.T("place.right"), Key: "r", Cmd: split("-h", false)},
		{Name: i18n.T("place.left"), Key: "l", Cmd: split("-h", true)},
		{Name: i18n.T("place.down"), Key: "d", Cmd: split("-v", false)},
		{Name: i18n.T("place.up"), Key: "u", Cmd: split("-v", true)},
		Separator(),
		{Name: i18n.T("place.tab"), Key: "t", Cmd: w.ui(append([]string{"tab", id}, uiTarget(p.ID, client)...)...)},
	}}
}

// ChangesMenu lists the target project's changed files (git status); each
// opens its diff in a popup. nil when nothing changed.
func (w *WS) ChangesMenu(ctx context.Context, p *Pane, client string) *Menu {
	gi := gitinfo.Collect(ctx, p.CWD)
	if gi.State != gitinfo.OK {
		return nil
	}
	rows := sidebar.ParseStatus(gi.Status)
	if len(rows) == 0 {
		return nil
	}
	title := i18n.T("sidebar.changes")
	if gi.Branch != "" {
		title += " · " + gi.Branch
	}
	m := &Menu{Title: title}
	for _, r := range rows {
		c := "display-popup"
		if client != "" {
			c += " -c " + tmux.Quote(client)
		}
		c += " -E -w 90% -h 90% -d " + tmux.Quote(p.CWD) + " " + tmux.Quote(sidebar.DiffShell(w.Self, r.Status, r.Path))
		m.Items = append(m.Items, MenuItem{Name: r.Status + "  " + r.Path, Cmd: c})
	}
	if more := gi.StatusTotal - len(rows); more > 0 {
		m.Items = append(m.Items, MenuItem{Name: i18n.T("menu.more", more), Disabled: true})
	}
	return m
}

// TabMenu asks which agent a new tab starts with.
func (w *WS) TabMenu(p *Pane, client string) *Menu {
	m := &Menu{Title: i18n.T("tab.title")}
	for _, a := range agent.Ordered(w.Cfg) {
		it := MenuItem{Name: a.Short, Key: a.Key, Cmd: w.ui(append([]string{"tab", string(a.ID)}, uiTarget(p.ID, client)...)...)}
		if !w.Installed(a.ID) {
			it.Name += " " + i18n.T("menu.not_installed")
			it.Disabled, it.Key = true, ""
		} else if until := w.LimitedUntil(a.ID); until != "" {
			it.Name += " " + i18n.T("sidebar.limited", until)
		}
		m.Items = append(m.Items, it)
	}
	m.Items = append(m.Items, Separator(), MenuItem{
		Name: i18n.T("menu.terminal"), Key: "t",
		Cmd: w.ui(append([]string{"tab", ShellAgent}, uiTarget(p.ID, client)...)...),
	})
	return m
}

// KeysMenu lists the keys (help).
func (w *WS) KeysMenu() *Menu {
	m := &Menu{Title: i18n.T("keys.title")}
	for _, k := range []string{"keys.menu", "keys.goto", "keys.switch", "keys.split", "keys.sidebar", "keys.sessions", "keys.tabs", "keys.panes", "keys.in_sidebar", "", "keys.prefix"} {
		if k == "" {
			m.Items = append(m.Items, Separator())
			continue
		}
		m.Items = append(m.Items, MenuItem{Name: i18n.T(k), Disabled: true})
	}
	return m
}

// ShowLimits opens the limits popup on client.
func (w *WS) ShowLimits(client string) error {
	c := tmux.Cmd{"display-popup"}
	if client != "" {
		c = append(c, "-c", client)
	}
	c = append(c, "-E", "-w", "80%", "-h", "60%", tmux.ShJoin(w.Self, "limits", "--pager"))
	return w.T.Spawn(c)
}

// ShowInstall opens the popup for agent id, which is not installed: what
// installs it, a button that does, then the chat in pane can continue in it
// (§9.3 "Not installed").
func (w *WS) ShowInstall(id session.AgentID, pane, client string) error {
	c := tmux.Cmd{"display-popup"}
	if client != "" {
		c = append(c, "-c", client)
	}
	args := append([]string{w.Self, "__install", string(id)}, uiTarget(pane, client)...)
	return w.T.Spawn(append(c, "-E", "-w", "72", "-h", "13", tmux.ShJoin(args...)))
}

// OptSidebarOff marks a window whose sidebar was turned off (prefix b): the
// watcher leaves it without one.
const OptSidebarOff = "@imux_sidebar_off"

// WindowsWithoutSidebar lists the windows that should have a sidebar and
// have none (it was killed: §9.3 "Sidebars come back"), each with the pane
// to add it next to: a live one first. None when sidebars are off in the
// config.
func (w *WS) WindowsWithoutSidebar(ctx context.Context) (map[string]string, error) {
	if !w.Cfg.Tmux.Sidebar {
		return nil, nil
	}
	out, err := w.T.Run(ctx, tmux.Cmd{
		"list-panes", "-a", "-F",
		"#{window_id}\t#{pane_id}\t#{" + sidebar.OptRole + "}\t#{" + OptSidebarOff + "}\t#{pane_dead}",
	})
	if err != nil {
		return nil, err
	}
	type win struct {
		bar, off   bool
		live, dead string
	}
	wins := map[string]*win{}
	for _, r := range tmux.Rows(out, 5) {
		x := wins[r[0]]
		if x == nil {
			x = &win{}
			wins[r[0]] = x
		}
		switch {
		case r[2] == sidebar.RoleSidebar:
			x.bar = true
		case r[4] == "1":
			if x.dead == "" {
				x.dead = r[1]
			}
		default:
			if x.live == "" {
				x.live = r[1]
			}
		}
		if r[3] == "1" {
			x.off = true
		}
	}
	missing := map[string]string{}
	for id, x := range wins {
		if x.bar || x.off {
			continue
		}
		target := x.live
		if target == "" {
			target = x.dead
		}
		if target != "" {
			missing[id] = target
		}
	}
	return missing, nil
}
