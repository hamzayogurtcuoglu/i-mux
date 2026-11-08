package workspace

import (
	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// Settings rows cycle through these values (§9.3 "Settings").
var (
	iconsCycle   = []string{"auto", "nerd", "emoji"}
	onLimitCycle = []string{"ask", "auto", "off"}
)

// labels name the values Settings shows.
func labels(v string) string {
	switch v {
	case "auto":
		return i18n.T("settings.icons_auto")
	case "nerd":
		return i18n.T("settings.icons_nerd")
	case "emoji":
		return i18n.T("settings.icons_emoji")
	case "wait":
		return i18n.T("settings.arrival_wait")
	case "continue":
		return i18n.T("settings.arrival_continue")
	case "ask":
		return i18n.T("settings.limit_ask")
	case "off":
		return i18n.T("settings.limit_off")
	}
	return v
}

// next is the value after cur in cycle.
func next(cycle []string, cur string) string {
	for i, v := range cycle {
		if v == cur {
			return cycle[(i+1)%len(cycle)]
		}
	}
	return cycle[0]
}

// settingsTarget is the --pane / --client tail of settings commands.
func settingsTarget(pane, client string) []string {
	var a []string
	if pane != "" {
		a = append(a, "--pane", pane)
	}
	if client != "" {
		a = append(a, "--client", client)
	}
	return a
}

// popupCmd runs imux with args in a popup with a terminal.
func (w *WS) popupCmd(width, height string, args ...string) string {
	return "display-popup -E -w " + width + " -h " + height + " " + tmux.Quote(tmux.ShJoin(append([]string{w.Self}, args...)...))
}

// SettingsMenu lists the common settings with their values; a row changes
// the config file and opens the menu again.
func (w *WS) SettingsMenu(pane, client string) *Menu {
	cfg := w.Cfg
	tgt := settingsTarget(pane, client)
	set := func(key, value string) string {
		return w.ui(append([]string{"__settings", "set", key, value}, tgt...)...)
	}
	m := &Menu{Title: i18n.T("settings.title")}
	if cfg.Completion.Enabled {
		m.Items = append(m.Items,
			MenuItem{Name: i18n.T("settings.completion", i18n.T("settings.on")), Key: "a", Cmd: set("completion", "off")},
			MenuItem{Name: i18n.T("settings.signin"), Key: "s", Cmd: w.popupCmd("90%", "90%", "__completion", "signin")})
	} else {
		m.Items = append(m.Items, MenuItem{
			Name: i18n.T("settings.completion", i18n.T("settings.off")), Key: "a",
			Cmd: w.popupCmd("90%", "80%", append([]string{"__completion", "setup"}, tgt...)...),
		})
	}
	icons := cfg.Tmux.Icons
	if icons == "" {
		icons = "auto"
	}
	commit := i18n.T("settings.agent_auto")
	if a := cfg.Git.CommitAgent; a != "" && a != "auto" {
		commit = session.DisplayName(session.AgentID(a))
	}
	arrival, other := "wait", "continue"
	if cfg.Handoff.OnArrival == "continue" {
		arrival, other = "continue", "wait"
	}
	m.Items = append(m.Items,
		MenuItem{Name: i18n.T("settings.icons", labels(icons)), Key: "i", Cmd: set("icons", next(iconsCycle, icons))},
		MenuItem{Name: i18n.T("settings.commit_agent", commit), Key: "m", Cmd: w.ui(append([]string{"__settings", "menu", "commit-agent"}, tgt...)...)},
		MenuItem{Name: i18n.T("settings.on_arrival", labels(arrival)), Key: "w", Cmd: set("on_arrival", other)},
		MenuItem{Name: i18n.T("settings.on_limit", limitLabel(cfg.OnLimit)), Key: "l", Cmd: set("on_limit", next(onLimitCycle, cfg.OnLimit))},
		Separator(),
		MenuItem{Name: i18n.T("settings.open"), Key: "o", Cmd: w.popupCmd("90%", "90%", "__settings", "edit")},
	)
	return m
}

// limitLabel names an on_limit value ("auto" is not the icons' Auto).
func limitLabel(v string) string {
	if v == "auto" {
		return i18n.T("settings.limit_auto")
	}
	return labels(v)
}

// CommitAgentMenu chooses who writes commit messages: auto or an installed
// agent.
func (w *WS) CommitAgentMenu(pane, client string) *Menu {
	tgt := settingsTarget(pane, client)
	cur := w.Cfg.Git.CommitAgent
	mark := func(v string) string {
		if v == cur || v == "auto" && cur == "" {
			return "● "
		}
		return "  "
	}
	m := &Menu{Title: i18n.T("settings.commit_agent_title")}
	m.Items = append(m.Items, MenuItem{
		Name: mark("auto") + i18n.T("settings.agent_auto"), Key: "0",
		Cmd: w.ui(append([]string{"__settings", "set", "commit_agent", "auto"}, tgt...)...),
	})
	for _, a := range agent.Ordered(w.Cfg) {
		if !w.Installed(a.ID) {
			continue
		}
		m.Items = append(m.Items, MenuItem{
			Name: mark(string(a.ID)) + a.Name, Key: a.Key,
			Cmd: w.ui(append([]string{"__settings", "set", "commit_agent", string(a.ID)}, tgt...)...),
		})
	}
	return m
}
