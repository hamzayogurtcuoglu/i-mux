package workspace

import (
	"context"
	"strconv"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
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
		for _, id := range bars {
			_, _ = w.T.Run(ctx, tmux.Cmd{"kill-pane", "-t", id})
		}
		return false, nil
	}
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
	return m
}

// KeysMenu lists the keys (help).
func (w *WS) KeysMenu() *Menu {
	m := &Menu{Title: i18n.T("keys.title")}
	for _, k := range []string{"keys.menu", "keys.switch", "keys.split", "keys.sidebar", "keys.sessions", "keys.panes", "", "keys.prefix"} {
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
