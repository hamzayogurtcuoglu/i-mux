package workspace

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/theme"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/editor"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	upd "github.com/hamzayogurtcuoglu/i-mux/internal/update"
	"github.com/hamzayogurtcuoglu/i-mux/internal/version"
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
	return "display-popup -E -w " + width + " -h " + height + " " + tmux.QuoteProgram(append([]string{w.Self}, args...)...)
}

// SettingsMenu lists the settings in three groups under dimmed headings
// (D93), each row a short name and its value in an aligned column; a row
// opens a menu of its values (D50), whose choice changes the config file
// and opens Settings again.
func (w *WS) SettingsMenu(pane, client string) *Menu {
	cfg := w.Cfg
	tgt := settingsTarget(pane, client)
	menu := func(which string) string {
		return w.ui(append([]string{"__settings", "menu", which}, tgt...)...)
	}
	theme := ThemeTitle(cfg.Theme.Name)
	if cfg.Theme.AutoSwitch {
		theme += " · " + i18n.T("settings.auto")
	}
	updates := version.Version
	if latest := w.State().Latest; upd.Newer(latest, updates) {
		updates = "⬆ " + latest
	}
	type row struct{ key, name, value, cmd string }
	groups := []struct {
		heading string
		rows    []row
	}{
		{i18n.T("settings.group_appearance"), []row{
			{"t", i18n.T("settings.theme"), theme, menu("theme")},
			{"i", i18n.T("settings.icons"), labels(iconsValue(cfg.Tmux.Icons)), menu("icons")},
		}},
		{i18n.T("settings.group_agents"), []row{
			{"l", i18n.T("settings.on_limit"), limitLabel(cfg.OnLimit), menu("on_limit")},
			{"w", i18n.T("settings.on_arrival"), labels(arrivalValue(cfg.Handoff.OnArrival)), menu("on_arrival")},
			{"n", i18n.T("settings.notify"), notifyLabel(cfg.Notify.Enabled, cfg.Notify.Sound), menu("notifications")},
			{"y", i18n.T("settings.auto_approve"), autoApproveLabel(cfg.OnPermission), menu("on_permission")},
		}},
		{i18n.T("settings.group_editing"), []row{
			{"e", i18n.T("settings.editor"), w.editorLabel(), menu("editor")},
			{"v", i18n.T("settings.paste_images"), onOffLabel(cfg.Paste.Images), menu("paste_images")},
		}},
		{"", []row{
			{"u", i18n.T("settings.updates"), updates, w.popupCmd("90%", "80%", "versions", "--popup")},
			{"o", i18n.T("settings.open"), "", w.popupCmd("90%", "90%", "__settings", "edit")},
		}},
	}
	width := 0
	for _, g := range groups {
		for _, r := range g.rows {
			width = max(width, utf8.RuneCountInString(r.name))
		}
	}
	m := &Menu{Title: i18n.T("settings.title")}
	for i, g := range groups {
		if g.heading != "" {
			m.Items = append(m.Items, MenuItem{Name: g.heading, Disabled: true})
		} else if i > 0 {
			m.Items = append(m.Items, Separator())
		}
		for _, r := range g.rows {
			name := "  " + r.name
			if r.value != "" {
				name += strings.Repeat(" ", width-utf8.RuneCountInString(r.name)+2) + r.value
			}
			m.Items = append(m.Items, MenuItem{Name: name, Key: r.key, Cmd: r.cmd})
		}
	}
	return m
}

// notifyLabel is the Notifications row's value: On, Silent (no sound) or
// Off (D93).
func notifyLabel(on, sound bool) string {
	switch {
	case !on:
		return i18n.T("settings.off")
	case !sound:
		return i18n.T("settings.notify_silent")
	}
	return i18n.T("settings.on_plain")
}

// notifyValue is notifyLabel's value for `__settings set notifications`.
func notifyValue(on, sound bool) string {
	switch {
	case !on:
		return "off"
	case !sound:
		return "silent"
	}
	return "sound"
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

// autoApproveLabel is the Auto-approve row's value (D112): Off, or On ⚠.
func autoApproveLabel(v string) string {
	if v == "approve" {
		return i18n.T("settings.auto_approve_on")
	}
	return i18n.T("settings.off")
}

// autoApproveValue is autoApproveLabel's value for `__settings set
// on_permission`.
func autoApproveValue(v string) string {
	if v == "approve" {
		return "approve"
	}
	return "wait"
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
	case "notifications", "notify", "notify_sound":
		cs := []Choice{
			{"sound", i18n.T("settings.notify_sound_on")},
			{"silent", i18n.T("settings.notify_silent_row")},
			{"off", i18n.T("settings.off")},
		}
		return w.ChoiceMenu(i18n.T("settings.notify_title"), "notifications", notifyValue(cfg.Notify.Enabled, cfg.Notify.Sound), cs, pane, client)
	case "paste_images":
		return w.ChoiceMenu(i18n.T("settings.paste_title"), "paste_images", onOffValue(cfg.Paste.Images), onOff, pane, client)
	case "icons":
		var cs []Choice
		for _, v := range []string{"auto", "nerd", "emoji"} {
			cs = append(cs, Choice{v, labels(v)})
		}
		m := w.ChoiceMenu(i18n.T("settings.icons_title"), "icons", iconsValue(cfg.Tmux.Icons), cs, pane, client)
		// The Nerd Font comes with every install (D90); here it can be put
		// in place again, or for a terminal set up later (D93).
		m.Items = append(m.Items, Separator(),
			MenuItem{Name: "  " + i18n.T("settings.font"), Key: "f", Cmd: w.popupCmd("90%", "70%", "font", "install", "--popup")})
		return m
	case "on_arrival":
		cs := []Choice{{"wait", i18n.T("settings.arrival_wait_row")}, {"continue", i18n.T("settings.arrival_continue_row")}}
		return w.ChoiceMenu(i18n.T("settings.arrival_title"), "on_arrival", arrivalValue(cfg.Handoff.OnArrival), cs, pane, client)
	case "theme":
		return w.ThemeMenu(pane, client)
	case "hooks":
		return w.ChoiceMenu(i18n.T("settings.hooks_title"), "hooks", onOffValue(cfg.Hooks.Enabled), onOff, pane, client)
	case "theme_auto":
		return w.ChoiceMenu(i18n.T("settings.theme_auto_title"), "theme_auto", onOffValue(cfg.Theme.AutoSwitch), onOff, pane, client)
	case "on_limit":
		var cs []Choice
		for _, c := range [][2]string{{"ask", "settings.limit_ask_row"}, {"auto", "settings.limit_auto_row"}, {"off", "settings.limit_off_row"}} {
			cs = append(cs, Choice{c[0], i18n.T(c[1])})
		}
		return w.ChoiceMenu(i18n.T("settings.limit_title"), "on_limit", cfg.OnLimit, cs, pane, client)
	case "on_permission":
		cs := []Choice{{"wait", i18n.T("settings.auto_approve_wait_row")}, {"approve", i18n.T("settings.auto_approve_on_row")}}
		m := w.ChoiceMenu(i18n.T("settings.auto_approve_title"), "on_permission", autoApproveValue(cfg.OnPermission), cs, pane, client)
		m.Items = append(m.Items, Separator(),
			MenuItem{Name: "  " + i18n.T("settings.auto_approve_note", strings.Join(cfg.ApproveWordList(), ", ")), Disabled: true})
		return m
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
	if cur == "copilot" {
		// Signing in again, while Copilot is the editor (D93).
		m.Items = append(m.Items, MenuItem{Name: "  " + i18n.T("settings.signin"), Key: "s", Cmd: w.popupCmd("90%", "90%", "__completion", "signin")})
	}
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
	// Following the terminal's light/dark is a switch at the top (D93).
	follow, next := "  ", "on"
	if w.Cfg.Theme.AutoSwitch {
		follow, next = "✓ ", "off"
	}
	m.Items = append(m.Items, MenuItem{
		Name: follow + i18n.T("settings.theme_auto"), Key: "0",
		Cmd: w.ui(append([]string{"__settings", "set", "theme_auto", next}, tgt...)...),
	}, Separator())
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
