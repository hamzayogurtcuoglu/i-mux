package workspace

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/editor"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
)

func cmdOf(m *Menu, key string) string {
	for _, it := range m.Items {
		if it.Key == key {
			return it.Cmd
		}
	}
	return ""
}

// noMicro makes micro missing and no editor chosen (D45).
func noMicro(t *testing.T) {
	t.Helper()
	look, ours := editor.LookPath, editor.OurMicro
	editor.LookPath = func(string) (string, error) { return "", errors.New("no") }
	editor.OurMicro = func() string { return "" }
	t.Cleanup(func() { editor.LookPath, editor.OurMicro = look, ours })
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
}

// TestSettingsMenu (D50, D93): three groups under dimmed headings, each row
// a short name and its value in one column; a row opens a menu of its
// values; Updates and the config file open popups.
func TestSettingsMenu(t *testing.T) {
	i18n.SetLocale("en")
	w, _, _ := testWS(t)
	noMicro(t)
	m := w.SettingsMenu("%1", "c1")
	var names []string
	for _, it := range m.Items {
		n := it.Key + " " + it.Name
		if it.Disabled {
			n = "-" + it.Name
		}
		names = append(names, n)
	}
	got := strings.Join(names, "|")
	want := "-APPEARANCE|t   Theme            i-mux|i   Icons            Auto|" +
		"-AGENTS|l   At a limit       Ask|w   After a handoff  Wait|n   Notifications    On|y   Approve all      Off|" +
		"-EDITING|e   Editor           Auto (the viewer)|v   Image paste      On| |" +
		"u   Updates          dev|o   Config file…"
	if m.Title != "Settings" || got != want {
		t.Fatalf("menu\n got %s\nwant %s", got, want)
	}
	for key, which := range map[string]string{
		"e": "editor", "n": "notifications", "v": "paste_images", "i": "icons", "t": "theme",
		"w": "on_arrival", "l": "on_limit", "y": "auto_approve",
	} {
		if c := cmdOf(m, key); !strings.Contains(c, "__settings menu "+which+" --pane %1 --client c1") {
			t.Errorf("%s: %q does not open the %s menu", key, c, which)
		}
	}
	for key, want := range map[string]string{"o": "__settings edit", "u": "versions --popup"} {
		if c := cmdOf(m, key); !strings.HasPrefix(c, "display-popup -E") || !strings.Contains(c, want) {
			t.Errorf("%s: %q", key, c)
		}
	}
	for _, gone := range []string{"h", "f", "d", "a", "c", "s"} {
		if c := cmdOf(m, gone); c != "" {
			t.Errorf("a row %s left: %q", gone, c)
		}
	}
	// Values change the rows.
	w.Cfg.Completion.Enabled, w.Cfg.Theme.AutoSwitch, w.Cfg.Notify.Sound = true, true, false
	w.Cfg.Tmux.Icons, w.Cfg.OnLimit, w.Cfg.Handoff.OnArrival = "emoji", "off", "continue"
	w.Cfg.AutoApprove = true
	got = ""
	for _, it := range w.SettingsMenu("%1", "").Items {
		got += it.Name + "|"
	}
	for _, want := range []string{
		"Theme            i-mux · auto|", "Icons            Emoji|", "At a limit       Do nothing|",
		"After a handoff  Continue|", "Notifications    Silent|", "Editor           Neovim + Copilot|",
		"Approve all      On ⚠|",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("menu lacks %q:\n%s", want, got)
		}
	}
	w.Cfg.Notify.Enabled = false
	if got := w.SettingsMenu("", "").Items[6].Name; got != "  Notifications    Off" {
		t.Errorf("notifications off: %q", got)
	}
	i18n.SetLocale("tr")
	defer i18n.SetLocale("en")
	if m := w.SettingsMenu("", ""); m.Title != "Ayarlar" || m.Items[0].Name != "GÖRÜNÜM" {
		t.Errorf("Turkish: %q %q", m.Title, m.Items[0].Name)
	}
}

// TestSettingMenus (D50, D93): a row's menu lists its values, ● on the
// current one; each saves its value.
func TestSettingMenus(t *testing.T) {
	i18n.SetLocale("en")
	w, _, _ := testWS(t)
	rows := func(m *Menu) string {
		var r []string
		for _, it := range m.Items {
			r = append(r, it.Key+" "+it.Name)
		}
		return strings.Join(r, "|")
	}
	for _, c := range []struct {
		which, title, rows, cmd string
	}{
		{"notifications", "Notifications", "1 ● On, with sound|2   On, silent|3   Off", "__settings set notifications off --pane %1 --client c1"},
		{"paste_images", "Image paste (Ctrl+V)", "1 ● On|2   Off", "__settings set paste_images off"},
		{"on_arrival", "After a handoff", "1 ● Wait for me|2   Continue the task", "__settings set on_arrival continue"},
		{"on_limit", "At a limit", "1 ● Ask me|2   Switch to the next agent|3   Do nothing", "__settings set on_limit off"},
	} {
		m := w.SettingMenu(c.which, "%1", "c1")
		if m == nil || m.Title != c.title || rows(m) != c.rows {
			t.Errorf("%s: %q %q", c.which, m.Title, rows(m))
			continue
		}
		if last := m.Items[len(m.Items)-1]; !strings.Contains(last.Cmd, c.cmd) {
			t.Errorf("%s: last row %q lacks %q", c.which, last.Cmd, c.cmd)
		}
	}
	// Icons: its values, then the Nerd Font install (D93).
	m := w.SettingMenu("icons", "%1", "c1")
	if rows(m) != "1 ● Auto|2   Nerd Font|3   Emoji| |f   Install the Nerd Font…" || !strings.Contains(cmdOf(m, "f"), "font install --popup") {
		t.Errorf("icons: %q", rows(m))
	}
	// Silent notifications: the middle value.
	w.Cfg.Notify.Sound = false
	if r := rows(w.SettingMenu("notifications", "", "")); r != "1   On, with sound|2 ● On, silent|3   Off" {
		t.Errorf("silent: %q", r)
	}
	// Approve all (D109): Off and On ⚠, then a line saying when it applies.
	m = w.SettingMenu("auto_approve", "%1", "c1")
	if m.Title != "Approve all: agents never ask" ||
		rows(m) != "1 ● Off: each agent asks as usual|2   On ⚠: approve every tool call, edit and command| |   For agents started after this; running ones restart to get it" ||
		!strings.Contains(cmdOf(m, "2"), "__settings set auto_approve on --pane %1 --client c1") || !m.Items[3].Disabled {
		t.Errorf("approve all: %q %q", m.Title, rows(m))
	}
	w.Cfg.AutoApprove = true
	if r := rows(w.SettingMenu("auto_approve", "", "")); !strings.HasPrefix(r, "1   Off: each agent asks as usual|2 ● On ⚠") {
		t.Errorf("approve all on: %q", r)
	}
	if w.SettingMenu("nope", "", "") != nil {
		t.Error("a menu for an unknown row")
	}
}

// TestEditorMenu (D50): Auto, micro, nano, Neovim, Neovim + Copilot; a
// missing editor is got (or explained) in a popup; ● on the current one.
func TestEditorMenu(t *testing.T) {
	i18n.SetLocale("en")
	w, _, _ := testWS(t)
	noMicro(t)
	have := map[string]bool{"nano": true}
	editor.LookPath = func(n string) (string, error) {
		if have[n] {
			return "/usr/bin/" + n, nil
		}
		return "", errors.New("no")
	}
	names := func(m *Menu) string {
		var r []string
		for _, it := range m.Items {
			r = append(r, it.Key+" "+it.Name)
		}
		return strings.Join(r, "|")
	}
	m := w.EditorMenu("%1", "c1")
	if m.Title != "Editor" || names(m) != "a ● Auto|m   micro · download (5 MB)|n   nano|v   Neovim| |c   Neovim + Copilot (AI)" {
		t.Fatalf("menu %q", names(m))
	}
	for key, want := range map[string]string{
		"a": "__settings set editor auto --pane %1 --client c1",
		"m": "editor use micro --popup", // missing: got in a popup
		"n": "__settings set editor nano",
		"v": "editor use nvim --popup", // missing: explained in a popup
		"c": "__completion setup --pane %1 --client c1",
	} {
		if c := cmdOf(m, key); !strings.Contains(c, want) {
			t.Errorf("%s: %q lacks %q", key, c, want)
		}
	}
	if w.editorLabel() != "Auto (nano)" {
		t.Errorf("label %q", w.editorLabel())
	}
	have["nvim"] = true
	editor.OurMicro = func() string { return "/home/u/.imux/tools/micro-2.0.15/micro" }
	w.Cfg.Editor = "nvim"
	m = w.EditorMenu("%1", "c1")
	if !strings.Contains(names(m), "v ● Neovim") || !strings.Contains(cmdOf(m, "m"), "__settings set editor micro") || !strings.Contains(cmdOf(m, "v"), "__settings set editor nvim") {
		t.Errorf("nvim chosen, micro there: %q", names(m))
	}
	w.Cfg.Completion.Enabled = true
	if c := EditorChoice(w.Cfg); c != "copilot" || !strings.Contains(cmdOf(w.EditorMenu("", ""), "c"), "__settings set editor copilot") {
		t.Errorf("copilot: %s", c)
	}
	// While Copilot is the editor, its sign-in is here (D93).
	if c := cmdOf(w.EditorMenu("", ""), "s"); !strings.Contains(c, "__completion signin") {
		t.Errorf("copilot sign-in: %q", c)
	}
	w.Cfg.Completion.Enabled, w.Cfg.Editor = false, "hx --flag"
	if m := w.EditorMenu("", ""); EditorChoice(w.Cfg) != "custom" || !m.Items[0].Disabled || m.Items[0].Name != "● hx --flag" || w.editorLabel() != "hx --flag" {
		t.Errorf("custom: %+v", m.Items[0])
	}
}

// TestWindowsWithoutSidebar (T41.1): a window whose sidebar was killed is
// listed with a live pane to add it to (a dead one when that is all); one
// with a sidebar, or turned off, is not; nothing when sidebars are off.
func TestWindowsWithoutSidebar(t *testing.T) {
	w, f, _ := testWS(t)
	w.Cfg.Tmux.Sidebar = true
	f.replies["list-panes"] = "@1\t%1\tsidebar\t\t0\n@1\t%2\t\t\t0\n" + // has one
		"@2\t%3\t\t\t1\n@2\t%4\t\t\t0\n" + // killed: the live pane
		"@3\t%5\t\t\t1\n" + // only a dead agent pane
		"@4\t%6\t\t1\t0\n" // turned off
	got, err := w.WindowsWithoutSidebar(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"@2": "%4", "@3": "%5"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	w.Cfg.Tmux.Sidebar = false
	if got, _ := w.WindowsWithoutSidebar(context.Background()); len(got) != 0 {
		t.Fatalf("sidebars off in the config: %v", got)
	}
}

// "+" → Web page… asks for the page to open (D47), starting with the last
// one.
func TestWebPrompt(t *testing.T) {
	i18n.SetLocale("en")
	w, f, _ := testWS(t)
	f.replies["show-options"] = "http://localhost:5173\n"
	if c := w.WebPrompt("%1", "c1"); !strings.Contains(c, "set-option -g @imux_web_pane %1 ; set-option -g @imux_web_client c1 ; command-prompt -t c1 -I http://localhost:5173") ||
		!strings.Contains(c, `set-option -g @imux_web_url "%%%"`) || !strings.Contains(c, "web open --from-option") {
		t.Errorf("open a page: %s", c)
	}
	// A page with a comma or # would not survive -I: the prompt starts empty.
	f.replies["show-options"] = "http://x/#a,b\n"
	if c := w.WebPrompt("%1", "c1"); !strings.Contains(c, "-I '' -p") {
		t.Errorf("odd page: %s", c)
	}
}

// A VS Code terminal's BROWSER helper is kept at attach (D47); a BROWSER
// of one's own (a terminal browser) or one without VS Code's socket is not.
func TestVSCodeBrowser(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	helper := "/home/u/.vscode-server/cli/servers/Stable-abc/server/bin/helpers/browser.sh"
	if b, i := VSCodeBrowser(env(map[string]string{"BROWSER": helper, "VSCODE_IPC_HOOK_CLI": "/tmp/vscode-ipc-1.sock"})); b != helper || i != "/tmp/vscode-ipc-1.sock" {
		t.Errorf("VS Code: %q %q", b, i)
	}
	for _, m := range []map[string]string{
		{"BROWSER": "w3m", "VSCODE_IPC_HOOK_CLI": "/tmp/s"},
		{"BROWSER": helper},
		{},
	} {
		if b, i := VSCodeBrowser(env(m)); b != "" || i != "" {
			t.Errorf("%v: %q %q", m, b, i)
		}
	}
}

// TestThemeMenu (T73.5): the 18 themes with keys 1-9 then letters, ● on
// the configured one (the default's for an unknown name); each sets it.
func TestThemeMenu(t *testing.T) {
	i18n.SetLocale("en")
	w, _, _ := testWS(t)
	m := w.ThemeMenu("%1", "c1")
	// The light/dark switch first (D93), then the themes.
	if !strings.Contains(m.Items[0].Cmd, "__settings set theme_auto on --pane %1 --client c1") || m.Items[0].Name != "  Follow the terminal's light/dark" || m.Items[1].Name != "" {
		t.Fatalf("follow: %+v", m.Items[0])
	}
	themes := m.Items[2:]
	if m.Title != "Theme" || len(themes) != 19 || themes[0].Name != "● i-mux" || themes[0].Key != "1" ||
		themes[9].Key != "a" || themes[18].Name != "  Vesper" || themes[18].Key != "j" {
		t.Fatalf("menu %+v", themes)
	}
	if !strings.Contains(themes[4].Cmd, "__settings set theme tokyo-night --pane %1 --client c1") {
		t.Errorf("tokyo-night: %q", themes[4].Cmd)
	}
	w.Cfg.Theme.AutoSwitch = true
	if f := w.ThemeMenu("", "").Items[0]; f.Name != "✓ Follow the terminal's light/dark" || !strings.Contains(f.Cmd, "theme_auto off") {
		t.Errorf("following: %+v", f)
	}
	w.Cfg.Theme.AutoSwitch = false
	w.Cfg.Theme.Name = "Rose Pine"
	for _, it := range w.ThemeMenu("", "").Items {
		if strings.HasPrefix(it.Name, "●") && it.Name != "● Rosé Pine" {
			t.Errorf("current %q", it.Name)
		}
	}
	if ThemeTitle("nope") != "i-mux" || ThemeTitle("tokyonight") != "Tokyo Night" {
		t.Error("ThemeTitle")
	}
}
