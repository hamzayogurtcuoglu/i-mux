package workspace

import (
	"context"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	upd "github.com/hamzayogurtcuoglu/i-mux/internal/update"
	"github.com/hamzayogurtcuoglu/i-mux/internal/version"
	"github.com/hamzayogurtcuoglu/i-mux/internal/versions"
	"github.com/hamzayogurtcuoglu/i-mux/internal/webview"
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
	cur := version.Version
	row := MenuItem{Name: i18n.T("settings.update", cur), Key: "u", Cmd: w.popupCmd("80%", "60%", "update", "--popup")}
	if latest := w.State().Latest; upd.Newer(latest, cur) {
		row.Name = i18n.T("settings.update_available", cur, latest)
	}
	// Versions and updates: i-mux, tmux, the agents; a tmux that lacks
	// features says so here.
	vers := MenuItem{Name: i18n.T("settings.versions"), Key: "c", Cmd: w.popupCmd("90%", "80%", "versions", "--popup")}
	if v, err := w.TmuxVersion(context.Background()); err == nil && !v.AtLeast(versions.TmuxWants[0], versions.TmuxWants[1]) {
		vers.Name = i18n.T("settings.versions_old", v.String())
	}
	m.Items = append(m.Items, row, vers, Separator())
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
	onOff := func(on bool) (string, string) {
		if on {
			return i18n.T("settings.on_plain"), "off"
		}
		return i18n.T("settings.off"), "on"
	}
	label, flip := onOff(cfg.Notify.Enabled)
	m.Items = append(m.Items, MenuItem{Name: i18n.T("settings.notify", label), Key: "n", Cmd: set("notify", flip)})
	if cfg.Notify.Enabled {
		label, flip = onOff(cfg.Notify.Sound)
		m.Items = append(m.Items, MenuItem{Name: i18n.T("settings.notify_sound", label), Key: "d", Cmd: set("notify_sound", flip)})
	}
	label, flip = onOff(cfg.Paste.Images)
	m.Items = append(m.Items, MenuItem{Name: i18n.T("settings.paste_images", label), Key: "v", Cmd: set("paste_images", flip)})
	web := i18n.T("settings.off")
	switch {
	case cfg.Web.Enabled && webview.Installed(cfg):
		web = i18n.T("settings.on_plain")
	case cfg.Web.Enabled:
		web = i18n.T("settings.web_not_installed")
	}
	m.Items = append(m.Items, MenuItem{Name: i18n.T("settings.web", web), Key: "p", Cmd: w.ui(append([]string{"__settings", "menu", "web"}, tgt...)...)})
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
		MenuItem{Name: i18n.T("settings.font"), Key: "f", Cmd: w.popupCmd("90%", "70%", "font", "install", "--popup")},
		MenuItem{Name: i18n.T("settings.commit_agent", commit), Key: "m", Cmd: w.ui(append([]string{"__settings", "menu", "commit-agent"}, tgt...)...)},
		MenuItem{Name: i18n.T("settings.on_arrival", labels(arrival)), Key: "w", Cmd: set("on_arrival", other)},
		MenuItem{Name: i18n.T("settings.on_limit", limitLabel(cfg.OnLimit)), Key: "l", Cmd: set("on_limit", next(onLimitCycle, cfg.OnLimit))},
		Separator(),
		MenuItem{Name: i18n.T("settings.open"), Key: "o", Cmd: w.popupCmd("90%", "90%", "__settings", "edit")},
	)
	return m
}

// WebMenu is Settings → Web preview (D44): install or remove its browser,
// turn the preview on or off, open a page.
func (w *WS) WebMenu(pane, client string) *Menu {
	tgt := settingsTarget(pane, client)
	cfg := w.Cfg
	installed := webview.Installed(cfg)
	m := &Menu{Title: i18n.T("settings.web_title")}
	if !installed && cfg.Web.Browser == "" {
		m.Items = append(m.Items, MenuItem{
			Name: i18n.T("settings.web_install", webview.DownloadMB), Key: "i",
			Cmd: w.popupCmd("80%", "60%", "web", "install", "--yes", "--popup"),
		})
	}
	if installed && cfg.Web.Enabled {
		m.Items = append(m.Items, MenuItem{Name: i18n.T("settings.web_open"), Key: "o", Cmd: w.WebPrompt(pane, client)})
	}
	if cfg.Web.Enabled {
		m.Items = append(m.Items, MenuItem{Name: i18n.T("settings.web_turn_off"), Key: "t", Cmd: w.ui(append([]string{"__settings", "set", "web", "off"}, tgt...)...)})
	} else {
		m.Items = append(m.Items, MenuItem{Name: i18n.T("settings.web_turn_on"), Key: "t", Cmd: w.ui(append([]string{"__settings", "set", "web", "on"}, tgt...)...)})
	}
	if installed && cfg.Web.Browser == "" {
		m.Items = append(m.Items, Separator(), MenuItem{
			Name: i18n.T("settings.web_remove", webview.DiskMB), Key: "r",
			Cmd: w.ui(append([]string{"web", "remove"}, tgt...)...),
		})
	}
	return m
}

// Options a web page's prompt leaves its answer and target in.
const (
	OptWebURL    = "@imux_web_url" // the page typed last
	OptWebPane   = "@imux_web_pane"
	OptWebClient = "@imux_web_client"
)

// WebPrompt asks on client for a page to open beside pane (D44), starting
// with the last one (http://localhost:3000 the first time). As in
// NamePrompt, nothing but the answer goes through the template.
func (w *WS) WebPrompt(pane, client string) string {
	last := "http://localhost:3000"
	if out, err := w.T.Run(context.Background(), tmux.Cmd{"show-options", "-gqv", OptWebURL}); err == nil {
		// -I is a comma-separated list and a format, expanded once more
		// by the menu: a page with either character starts empty.
		if v := strings.TrimSpace(out); v != "" {
			last = v
			if strings.ContainsAny(v, ",#") {
				last = ""
			}
		}
	}
	c := "set-option -g " + OptWebPane + " " + tmux.Quote(pane) +
		" ; set-option -g " + OptWebClient + " " + tmux.Quote(client) + " ; command-prompt"
	if client != "" {
		c += " -t " + tmux.Quote(client)
	}
	tmpl := "set-option -g " + OptWebURL + " \"%%%\" ; " + tmux.RunShell(w.Self, "web", "open", "--from-option")
	return c + " -I " + tmux.Quote(last) + " -p " + tmux.Quote(i18n.T("prompt.web")) + " " + tmux.Quote(tmpl)
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
