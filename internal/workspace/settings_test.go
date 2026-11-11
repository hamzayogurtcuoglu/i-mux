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
	want := "u Update i-mux… (dev)|c Versions and updates…| |a AI completion in the editor: Off|n Notifications: On|d Notification sound: On|v Paste images with Ctrl+V: On|p 🌐 HTML files open: in a window, live|i Icons: Auto|f Install the Nerd Font for the icons…|e Get the micro editor (5 MB)…|m Commit messages by: Auto (the pane's agent)|" +
		"w After a handoff the new agent: waits for you|l When an agent hits its limit: ask me| |o Open the config file…"
	if m.Title != "i-mux · Settings" || got != want {
		t.Fatalf("menu\n got %s\nwant %s", got, want)
	}
	cmd := func(key string) string {
		for _, it := range m.Items {
			if it.Key == key {
				return it.Cmd
			}
		}
		return ""
	}
	for key, want := range map[string]string{
		"a": "__completion setup --pane %1 --client c1",
		"i": "__settings set icons nerd --pane %1 --client c1",
		"f": "font install --popup",
		"e": "editor install --yes --popup",
		"m": "__settings menu commit-agent --pane %1 --client c1",
		"w": "__settings set on_arrival continue",
		"n": "__settings set notify off",
		"d": "__settings set notify_sound off",
		"v": "__settings set paste_images off",
		"l": "__settings set on_limit auto",
		"o": "__settings edit",
	} {
		if !strings.Contains(cmd(key), want) {
			t.Errorf("%s: %q lacks %q", key, cmd(key), want)
		}
	}
	if !strings.HasPrefix(cmd("a"), "display-popup -E") || !strings.HasPrefix(cmd("o"), "display-popup -E") || !strings.HasPrefix(cmd("f"), "display-popup -E") {
		t.Error("setup and editing run in popups")
	}

	if c := cmd("u"); !strings.HasPrefix(c, "display-popup -E") || !strings.Contains(c, "update --popup") {
		t.Errorf("update row %q", c)
	}
	// With micro there, or an editor chosen, there is nothing to get.
	t.Setenv("EDITOR", "vim")
	if c := cmdOf(w.SettingsMenu("%1", "c1"), "e"); c != "" {
		t.Errorf("micro offered with $EDITOR set: %q", c)
	}
	t.Setenv("EDITOR", "")
	editor.OurMicro = func() string { return "/home/u/.imux/tools/micro-2.0.15/micro" }
	if c := cmdOf(w.SettingsMenu("%1", "c1"), "e"); c != "" {
		t.Errorf("micro offered while installed: %q", c)
	}
	if c := cmd("c"); !strings.HasPrefix(c, "display-popup -E") || !strings.Contains(c, "versions --popup") {
		t.Errorf("versions row %q", c)
	}
	// A tmux that lacks features says so on the row (tmux -V's reply).
	for v, want := range map[string]string{
		"tmux 3.2a": "⚠ Versions and updates… (tmux 3.2a is old)",
		"tmux 3.4":  "⚠ Versions and updates… (tmux 3.4 is old)",
		"tmux 3.5a": "Versions and updates…",
	} {
		f.replies[""] = v
		for _, it := range w.SettingsMenu("%1", "c1").Items {
			if it.Key == "c" && it.Name != want {
				t.Errorf("%s: row %q, want %q", v, it.Name, want)
			}
		}
	}
	f.replies[""] = ""
	// Values change the rows; completion on offers turning off and signing in.
	w.Cfg.Completion.Enabled = true
	w.Cfg.Tmux.Icons, w.Cfg.OnLimit, w.Cfg.Handoff.OnArrival, w.Cfg.Git.CommitAgent = "emoji", "off", "continue", "opencode"
	m = w.SettingsMenu("%1", "")
	got = ""
	for _, it := range m.Items {
		got += it.Key + " " + it.Name + "|"
	}
	for _, want := range []string{
		"a AI completion in the editor: On (GitHub Copilot)|s Sign in to GitHub Copilot…|",
		"i Icons: Emoji|", "m Commit messages by: OpenCode|", "w After a handoff the new agent: continues at once|",
		"l When an agent hits its limit: do nothing|",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("menu lacks %q:\n%s", want, got)
		}
	}
	if c := cmdOf(m, "a"); !strings.Contains(c, "__settings set completion off") {
		t.Errorf("turning off: %q", c)
	}
	if c := cmdOf(m, "i"); !strings.Contains(c, "__settings set icons auto") {
		t.Errorf("icons cycle back to auto: %q", c)
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

// Settings → where HTML files open (D47): the row cycles a window, a tab,
// the editor; "+" → Web page… asks for the page to open, starting with the
// last one.
func TestWebSettingsAndPrompt(t *testing.T) {
	i18n.SetLocale("en")
	w, f, _ := testWS(t)
	row := func() MenuItem {
		for _, it := range w.SettingsMenu("%1", "c1").Items {
			if it.Key == "p" {
				return it
			}
		}
		t.Fatal("no web row")
		return MenuItem{}
	}
	for _, c := range []struct {
		enabled  bool
		open     string
		name, to string
	}{
		{true, "window", "🌐 HTML files open: in a window, live", "set web tab"},
		{true, "tab", "🌐 HTML files open: in a browser tab, live", "set web off"},
		{false, "tab", "🌐 HTML files open: in the editor", "set web window"},
	} {
		w.Cfg.Web.Enabled, w.Cfg.Web.Open = c.enabled, c.open
		if it := row(); it.Name != c.name || !strings.Contains(it.Cmd, "__settings "+c.to+" --pane %1 --client c1") {
			t.Errorf("%v %s: %+v", c.enabled, c.open, it)
		}
	}
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
