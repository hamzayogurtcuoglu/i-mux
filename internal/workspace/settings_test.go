package workspace

import (
	"context"
	"errors"
	"reflect"
	"runtime"
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

// TestSettingsMenu (D50): each row shows its value and opens a menu of
// its values; actions open popups.
func TestSettingsMenu(t *testing.T) {
	i18n.SetLocale("en")
	w, f, _ := testWS(t)
	noMicro(t)
	m := w.SettingsMenu("%1", "c1")
	var names []string
	for _, it := range m.Items {
		names = append(names, it.Key+" "+it.Name)
	}
	got := strings.Join(names, "|")
	want := "u Update i-mux… (dev)|c Versions and updates…| |e Editor: Auto (the viewer)|n Notifications: On|d Notification sound: On|v Paste images with Ctrl+V: On|i Icons: Auto|f Install the Nerd Font for the icons…|m Commit messages by: Auto (the pane's agent)|" +
		"w After a handoff the new agent: waits for you|l When an agent hits its limit: ask me| |o Open the config file…"
	if m.Title != "i-mux · Settings" || got != want {
		t.Fatalf("menu\n got %s\nwant %s", got, want)
	}
	for key, which := range map[string]string{
		"e": "editor", "n": "notify", "d": "notify_sound", "v": "paste_images", "i": "icons",
		"m": "commit-agent", "w": "on_arrival", "l": "on_limit",
	} {
		if c := cmdOf(m, key); !strings.Contains(c, "__settings menu "+which+" --pane %1 --client c1") {
			t.Errorf("%s: %q does not open the %s menu", key, c, which)
		}
	}
	for key, want := range map[string]string{"f": "font install --popup", "o": "__settings edit", "u": "update --popup", "c": "versions --popup"} {
		if c := cmdOf(m, key); !strings.HasPrefix(c, "display-popup -E") || !strings.Contains(c, want) {
			t.Errorf("%s: %q", key, c)
		}
	}
	// A tmux that lacks features says so on the row (tmux -V's reply); 3.4
	// has them all (menus take the mouse on every tmux, D54).
	olds := map[string]string{
		"tmux 3.2a": "⚠ Versions and updates… (tmux 3.2a is old)",
		"tmux 3.3a": "⚠ Versions and updates… (tmux 3.3a is old)",
		"tmux 3.4":  "Versions and updates…",
		"tmux 3.5a": "Versions and updates…",
	}
	if runtime.GOOS == "windows" {
		// psmux draws its menus itself: it is old only below tmux.MinVersion.
		olds = map[string]string{
			"tmux 3.5a": "Versions and updates…",
			"tmux 3.2a": "Versions and updates…",
			"tmux 3.4":  "Versions and updates…",
		}
	}
	for v, want := range olds {
		f.replies[""] = v
		for _, it := range w.SettingsMenu("%1", "c1").Items {
			if it.Key == "c" && it.Name != want {
				t.Errorf("%s: row %q, want %q", v, it.Name, want)
			}
		}
	}
	f.replies[""] = ""
	// Values change the rows; with AI completion on, Copilot's sign-in.
	w.Cfg.Completion.Enabled = true
	w.Cfg.Tmux.Icons, w.Cfg.OnLimit, w.Cfg.Handoff.OnArrival, w.Cfg.Git.CommitAgent = "emoji", "off", "continue", "opencode"
	m = w.SettingsMenu("%1", "")
	got = ""
	for _, it := range m.Items {
		got += it.Key + " " + it.Name + "|"
	}
	for _, want := range []string{
		"e Editor: Neovim + Copilot|s Sign in to GitHub Copilot…|",
		"i Icons: Emoji|", "m Commit messages by: OpenCode|", "w After a handoff the new agent: continues at once|",
		"l When an agent hits its limit: do nothing|",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("menu lacks %q:\n%s", want, got)
		}
	}

	sub := w.CommitAgentMenu("%1", "")
	if sub.Title != "Commit messages by" || !strings.HasPrefix(sub.Items[0].Name, "  Auto") {
		t.Fatalf("submenu %+v", sub.Items)
	}
	var marked []string
	for _, it := range sub.Items {
		if strings.HasPrefix(it.Name, "● ") {
			marked = append(marked, it.Name)
		}
	}
	if len(marked) != 0 { // opencode is not installed in testWS
		t.Errorf("marked %v", marked)
	}
	if !strings.Contains(sub.Items[1].Cmd, "__settings set commit_agent claude") {
		t.Errorf("agent row %+v", sub.Items[1])
	}
}

// TestSettingMenus (D50): a row's menu lists its values, ● on the current
// one; each saves its value.
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
		{"notify", "Notifications", "1 ● On|2   Off", "__settings set notify off --pane %1 --client c1"},
		{"notify_sound", "Notification sound", "1 ● On|2   Off", "__settings set notify_sound off"},
		{"paste_images", "Paste images with Ctrl+V", "1 ● On|2   Off", "__settings set paste_images off"},
		{"icons", "Icons", "1 ● Auto|2   Nerd Font|3   Emoji", "__settings set icons emoji"},
		{"on_arrival", "After a handoff the new agent", "1 ● waits for you|2   continues at once", "__settings set on_arrival continue"},
		{"on_limit", "When an agent hits its limit", "1 ● ask me|2   switch on its own|3   do nothing", "__settings set on_limit off"},
	} {
		m := w.SettingMenu(c.which, "%1", "c1")
		if m == nil || m.Title != c.title || rows(m) != c.rows {
			t.Errorf("%s: %+v", c.which, m)
			continue
		}
		if last := m.Items[len(m.Items)-1]; !strings.Contains(last.Cmd, c.cmd) {
			t.Errorf("%s: last row %q lacks %q", c.which, last.Cmd, c.cmd)
		}
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
	if m.Title != "Files open in" || names(m) != "a ● Auto · micro, $VISUAL, $EDITOR, nano|m   micro · get it (5 MB)|n   nano|v   Neovim| |c   Neovim + Copilot suggestions (AI completion)" {
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
