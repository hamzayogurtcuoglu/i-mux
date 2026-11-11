package workspace

import (
	"context"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/textfmt"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// MenuItem is one display-menu entry. An item with an empty Name is a
// separator.
type MenuItem struct {
	Name     string // plain text (escaped for tmux formats when shown)
	Key      string // mnemonic ("" = none)
	Cmd      string // tmux command run when chosen
	Disabled bool
}

// Menu is a display-menu.
type Menu struct {
	Title string
	Items []MenuItem
}

// Separator returns a separator line.
func Separator() MenuItem { return MenuItem{} }

// Cmd returns the display-menu command for client and pane.
func (m *Menu) Cmd(client, pane string) tmux.Cmd {
	c := tmux.Cmd{"display-menu"}
	if client != "" {
		c = append(c, "-c", client)
	}
	if pane != "" {
		c = append(c, "-t", pane)
	}
	// "--" ends the options: a disabled first item ("-Claude: limited …")
	// would otherwise be parsed as flags.
	c = append(c, "-T", tmux.EscapeFormat(m.Title), "-x", "C", "-y", "C", "--")
	for i, it := range m.Items {
		if it.Name == "" {
			// No separator at the very start or end, and never two in a row.
			if i == 0 || i == len(m.Items)-1 || m.Items[i-1].Name == "" {
				continue
			}
			c = append(c, "")
			continue
		}
		name := tmux.EscapeFormat(it.Name)
		if it.Disabled {
			name = "-" + name
		}
		c = append(c, name, it.Key, it.Cmd)
	}
	return c
}

// Show displays the menu. display-menu keeps its tmux client waiting until
// the menu closes, so that client is spawned and not waited for.
func (w *WS) Show(ctx context.Context, m *Menu, client, pane string) error {
	c := m.Cmd(client, pane)
	return w.T.ShowMenu(ctx, append(w.T.Menu(ctx), c[1:]...))
}

// ui returns the tmux command that runs imux silently with args.
func (w *WS) ui(args ...string) string { return tmux.RunShell(w.Self, args...) }

// UICommand is ui for other packages (the watcher's prompts).
func (w *WS) UICommand(args ...string) string { return w.ui(args...) }

// uiTarget returns the --pane/--client arguments.
func uiTarget(pane, client string) []string {
	a := []string{"--pane", pane}
	if client != "" {
		a = append(a, "--client", client)
	}
	return a
}

// LimitedUntil returns "15:00" when agent id is limited now ("" otherwise).
func (w *WS) LimitedUntil(id session.AgentID) string {
	l := w.State().ActiveLimit(string(id), w.Now())
	if l == nil {
		return ""
	}
	t := l.ResetAt
	if t.IsZero() {
		t = l.ExpiresAt
	}
	return textfmt.Clock(w.Now(), t)
}

// PaneMenu builds the pane menu (§9.3) for pane p opened on client.
func (w *WS) PaneMenu(p *Pane, client string) *Menu {
	cur := p.Agent
	if p.Dead {
		cur = ""
	}
	m := &Menu{Title: i18n.T("menu.title", i18n.T("menu.shell"))}
	if cur != "" {
		m.Title = i18n.T("menu.title", session.ShortName(cur))
	}
	tgt := uiTarget(p.ID, client)
	if cur != "" {
		if until := w.LimitedUntil(cur); until != "" {
			m.Items = append(m.Items, MenuItem{Name: i18n.T("menu.limited_title", session.ShortName(cur), until), Disabled: true}, Separator())
		}
	}
	for _, a := range agent.Ordered(w.Cfg) {
		if a.ID == cur {
			continue
		}
		it := MenuItem{Key: a.Key, Cmd: w.ui(append([]string{"switch", string(a.ID)}, tgt...)...)}
		if cur == "" {
			it.Name = i18n.T("menu.start", a.Short)
		} else {
			it.Name = i18n.T("menu.switch", a.Short)
		}
		switch {
		case !w.Installed(a.ID):
			it.Name += " " + i18n.T("menu.not_installed")
			it.Disabled = true
		case w.LimitedUntil(a.ID) != "":
			it.Name += " " + i18n.T("menu.limited_suffix", w.LimitedUntil(a.ID))
		}
		m.Items = append(m.Items, it)
	}
	m.Items = append(m.Items,
		MenuItem{Name: i18n.T("menu.sessions"), Key: "s", Cmd: w.ui(append([]string{"__menu", "--sessions"}, tgt...)...)},
		Separator())
	for _, a := range agent.Ordered(w.Cfg) {
		if !w.Installed(a.ID) {
			continue
		}
		m.Items = append(m.Items, MenuItem{
			Name: i18n.T("menu.new_pane", a.Short), Key: strings.ToUpper(a.Key),
			Cmd: w.ui(append([]string{"split", string(a.ID), "-h"}, tgt...)...),
		})
	}
	m.Items = append(m.Items, Separator(),
		MenuItem{Name: i18n.T("menu.preview"), Key: "v", Cmd: w.previewCmd(p, ""), Disabled: cur == "" && p.Trail.Last() == nil},
		MenuItem{Name: i18n.T("menu.limits"), Key: "l", Cmd: w.limitsCmd()},
		Separator(),
		MenuItem{Name: i18n.T("menu.close"), Key: "k", Cmd: "kill-pane -t " + p.ID},
	)
	return m
}

// previewCmd opens the handoff preview popup for a pane (or a picked
// conversation with from = "agent:id").
func (w *WS) previewCmd(p *Pane, from string) string {
	args := []string{w.Self, "handoff", "--pane", p.ID, "--print"}
	if from != "" {
		args = []string{w.Self, "handoff", "--session", from, "--cwd", p.CWD, "--print"}
	}
	sh := pagedCmd(args)
	// Menu items run on the client that chose them: no -c (tmux 3.2a
	// rejects -c inside menu item commands).
	c := "display-popup -E -w 90% -h 90%"
	if p.CWD != "" {
		c += " -d " + tmux.Quote(p.CWD)
	}
	return c + " " + tmux.Quote(sh)
}

// limitsPanel: `imux limits` exists (M4); false shows "coming soon".
var limitsPanel = true

// limitsCmd opens the limits panel; until M4 it says it is coming.
func (w *WS) limitsCmd() string {
	if limitsPanel {
		sh := tmux.ShJoin(w.Self, "limits", "--pager")
		c := "display-popup -E -w 80% -h 60%"
		return c + " " + tmux.Quote(sh)
	}
	return "display-message " + tmux.Quote(tmux.EscapeMessage(i18n.T("menu.limits_soon")))
}
