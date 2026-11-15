package workspace

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/theme"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/editor"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	upd "github.com/hamzayogurtcuoglu/i-mux/internal/update"
	"github.com/hamzayogurtcuoglu/i-mux/internal/version"
	"github.com/hamzayogurtcuoglu/i-mux/internal/versions"
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

// SettingsMenu lists the common settings with their values; a row opens a
// menu of its values (D50), whose choice changes the config file and opens
// Settings again.
func (w *WS) SettingsMenu(pane, client string) *Menu {
	cfg := w.Cfg
	tgt := settingsTarget(pane, client)
	menu := func(which string) string {
		return w.ui(append([]string{"__settings", "menu", which}, tgt...)...)
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
	m.Items = append(m.Items, row, vers, Separator(),
		MenuItem{Name: i18n.T("settings.editor", w.editorLabel()), Key: "e", Cmd: menu("editor")})
	if cfg.Completion.Enabled {
		m.Items = append(m.Items, MenuItem{Name: i18n.T("settings.signin"), Key: "s", Cmd: w.popupCmd("90%", "90%", "__completion", "signin")})
	}
	m.Items = append(m.Items, MenuItem{Name: i18n.T("settings.notify", onOffLabel(cfg.Notify.Enabled)), Key: "n", Cmd: menu("notify")})
	if cfg.Notify.Enabled {
		m.Items = append(m.Items, MenuItem{Name: i18n.T("settings.notify_sound", onOffLabel(cfg.Notify.Sound)), Key: "d", Cmd: menu("notify_sound")})
	}
	m.Items = append(m.Items,
		MenuItem{Name: i18n.T("settings.paste_images", onOffLabel(cfg.Paste.Images)), Key: "v", Cmd: menu("paste_images")},
		MenuItem{Name: i18n.T("settings.icons", labels(iconsValue(cfg.Tmux.Icons))), Key: "i", Cmd: menu("icons")},
		MenuItem{Name: i18n.T("settings.font"), Key: "f", Cmd: w.popupCmd("90%", "70%", "font", "install", "--popup")},
		MenuItem{Name: i18n.T("settings.theme", ThemeTitle(cfg.Theme.Name)), Key: "t", Cmd: menu("theme")},
		MenuItem{Name: i18n.T("settings.theme_auto", onOffLabel(cfg.Theme.AutoSwitch)), Key: "a", Cmd: menu("theme_auto")},
		MenuItem{Name: i18n.T("settings.on_arrival", labels(arrivalValue(cfg.Handoff.OnArrival))), Key: "w", Cmd: menu("on_arrival")},
		MenuItem{Name: i18n.T("settings.on_limit", limitLabel(cfg.OnLimit)), Key: "l", Cmd: menu("on_limit")},
		MenuItem{Name: i18n.T("settings.hooks", onOffLabel(cfg.Hooks.Enabled)), Key: "h", Cmd: menu("hooks")},
		Separator(),
		MenuItem{Name: i18n.T("settings.open"), Key: "o", Cmd: w.popupCmd("90%", "90%", "__settings", "edit")},
	)
	return m
}

func onOffLabel(on bool) string {
	if on {
		return i18n.T("settings.on_plain")
	}
	return i18n.T("settings.off")
}

func onOffValue(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

func iconsValue(v string) string {
	if v == "" {
		return "auto"
	}
	return v
}

func arrivalValue(v string) string {
	if v == "continue" {
		return v
	}
	return "wait"
}

// Choice is one value of a setting: what is saved, and how it reads.
type Choice struct{ Value, Label string }

// ChoiceMenu lists a setting's values with ● on the current one; choosing
// one saves it (`imux __settings set key value`) and opens Settings again.
func (w *WS) ChoiceMenu(title, key, cur string, cs []Choice, pane, client string) *Menu {
	tgt := settingsTarget(pane, client)
	m := &Menu{Title: title}
	for i, c := range cs {
		mark := "  "
		if c.Value == cur {
			mark = "● "
		}
		m.Items = append(m.Items, MenuItem{
			Name: mark + c.Label, Key: strconv.Itoa(i + 1),
			Cmd: w.ui(append([]string{"__settings", "set", key, c.Value}, tgt...)...),
		})
	}
	return m
}

// SettingMenu is the menu of Settings row which (nil for none).
func (w *WS) SettingMenu(which, pane, client string) *Menu {
	cfg := w.Cfg
	onOff := []Choice{{"on", i18n.T("settings.on_plain")}, {"off", i18n.T("settings.off")}}
	switch which {
	case "editor":
		return w.EditorMenu(pane, client)
	case "notify":
		return w.ChoiceMenu(i18n.T("settings.notify_title"), "notify", onOffValue(cfg.Notify.Enabled), onOff, pane, client)
	case "notify_sound":
		return w.ChoiceMenu(i18n.T("settings.sound_title"), "notify_sound", onOffValue(cfg.Notify.Sound), onOff, pane, client)
	case "paste_images":
		return w.ChoiceMenu(i18n.T("settings.paste_title"), "paste_images", onOffValue(cfg.Paste.Images), onOff, pane, client)
	case "icons":
		var cs []Choice
		for _, v := range []string{"auto", "nerd", "emoji"} {
			cs = append(cs, Choice{v, labels(v)})
		}
		return w.ChoiceMenu(i18n.T("settings.icons_title"), "icons", iconsValue(cfg.Tmux.Icons), cs, pane, client)
	case "on_arrival":
		cs := []Choice{{"wait", labels("wait")}, {"continue", labels("continue")}}
		return w.ChoiceMenu(i18n.T("settings.arrival_title"), "on_arrival", arrivalValue(cfg.Handoff.OnArrival), cs, pane, client)
	case "theme":
		return w.ThemeMenu(pane, client)
	case "hooks":
		return w.ChoiceMenu(i18n.T("settings.hooks_title"), "hooks", onOffValue(cfg.Hooks.Enabled), onOff, pane, client)
	case "theme_auto":
		return w.ChoiceMenu(i18n.T("settings.theme_auto_title"), "theme_auto", onOffValue(cfg.Theme.AutoSwitch), onOff, pane, client)
	case "on_limit":
		var cs []Choice
		for _, v := range []string{"ask", "auto", "off"} {
			cs = append(cs, Choice{v, limitLabel(v)})
		}
		return w.ChoiceMenu(i18n.T("settings.limit_title"), "on_limit", cfg.OnLimit, cs, pane, client)
	}
	return nil
}

// EditorChoice is the Editor row's value (D50): copilot (AI completion,
// D30), auto, micro, nano, nvim, or custom (another editor in the config).
func EditorChoice(cfg *config.Config) string {
	if cfg.Completion.Enabled {
		return "copilot"
	}
	f := strings.Fields(cfg.Editor)
	if len(f) == 0 {
		return "auto"
	}
	switch b := filepath.Base(f[0]); b {
	case "micro", "nano", "nvim":
		return b
	}
	return "custom"
}

// editorLabel is the Editor row's value as it reads: "Auto (nano)",
// "micro", "Neovim + Copilot", or the configured command.
func (w *WS) editorLabel() string {
	switch c := EditorChoice(w.Cfg); c {
	case "auto":
		name := i18n.T("settings.editor_viewer")
		if ed := editor.Choose(""); ed != nil {
			name = filepath.Base(ed[0])
		}
		return i18n.T("settings.editor_auto", name)
	case "nvim":
		return "Neovim"
	case "copilot":
		return i18n.T("settings.editor_copilot")
	case "custom":
		return w.Cfg.Editor
	default:
		return c
	}
}

// EditorMenu chooses the editor files open in (D50): Auto, micro (got
// first when missing, D45), nano, Neovim, Neovim + Copilot (AI completion,
// D30). An editor that is missing opens `imux editor use` in a popup, which
// gets it or says how; a configured command of one's own is shown first.
func (w *WS) EditorMenu(pane, client string) *Menu {
	tgt := settingsTarget(pane, client)
	cur := EditorChoice(w.Cfg)
	mark := func(v string) string {
		if v == cur {
			return "● "
		}
		return "  "
	}
	set := func(v string) string { return w.ui(append([]string{"__settings", "set", "editor", v}, tgt...)...) }
	use := func(v string) string { return w.popupCmd("80%", "60%", "editor", "use", v, "--popup") }
	have := func(bin string) bool { _, err := editor.LookPath(bin); return err == nil }
	m := &Menu{Title: i18n.T("settings.editor_title")}
	if cur == "custom" {
		m.Items = append(m.Items, MenuItem{Name: mark("custom") + w.Cfg.Editor, Disabled: true})
	}
	auto := i18n.T("settings.editor_auto_row")
	m.Items = append(m.Items, MenuItem{Name: mark("auto") + auto, Key: "a", Cmd: set("auto")})
	micro := MenuItem{Name: mark("micro") + i18n.T("settings.editor_micro"), Key: "m", Cmd: set("micro")}
	if !editor.HasMicro() {
		micro.Name, micro.Cmd = mark("micro")+i18n.T("settings.editor_micro_get", editor.MicroMB), use("micro")
	}
	nano := MenuItem{Name: mark("nano") + "nano", Key: "n", Cmd: set("nano")}
	if !have("nano") {
		nano.Cmd = use("nano")
	}
	nvim := MenuItem{Name: mark("nvim") + "Neovim", Key: "v", Cmd: set("nvim")}
	if !have("nvim") {
		nvim.Cmd = use("nvim")
	}
	copilot := MenuItem{Name: mark("copilot") + i18n.T("settings.editor_copilot_row"), Key: "c", Cmd: set("copilot")}
	if cur != "copilot" {
		// Setting it up installs the plugin and signs in (D30).
		copilot.Cmd = w.popupCmd("90%", "80%", append([]string{"__completion", "setup"}, tgt...)...)
	}
	m.Items = append(m.Items, micro, nano, nvim, Separator(), copilot)
	return m
}

// Options a web page's prompt leaves its answer and target in.
const (
	OptWebURL    = "@imux_web_url" // the page typed last
	OptWebPane   = "@imux_web_pane"
	OptWebClient = "@imux_web_client"
)

// WebPrompt asks on client for a page to open in the browser (D47): an
// address, or a file of pane's project; it starts with the last one
// (http://localhost:3000 the first time). As in NamePrompt, nothing but
// the answer goes through the template.
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

// ThemeTitle is a theme's name as menus show it ("Catppuccin"); an unknown
// one is the default's.
func ThemeTitle(name string) string {
	n, ok := theme.Canonical(name)
	if !ok {
		n = theme.Default
	}
	return theme.Titles[n]
}

// themeKeys are the theme menu's keys: 1-9, then letters.
const themeKeys = "123456789abcdefghijklmnopq"

// ThemeMenu lists the themes (D68), ● on the current one; choosing one sets
// it and stops following the terminal's light/dark, as a manual choice does.
func (w *WS) ThemeMenu(pane, client string) *Menu {
	tgt := settingsTarget(pane, client)
	cur, _ := theme.Canonical(w.Cfg.Theme.Name)
	if cur == "" {
		cur = theme.Default
	}
	m := &Menu{Title: i18n.T("settings.theme_title")}
	for i, n := range theme.Names {
		mark := "  "
		if n == cur {
			mark = "● "
		}
		m.Items = append(m.Items, MenuItem{
			Name: mark + theme.Titles[n], Key: string(themeKeys[i]),
			Cmd: w.ui(append([]string{"__settings", "set", "theme", n}, tgt...)...),
		})
	}
	return m
}
