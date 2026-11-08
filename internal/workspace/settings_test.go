package workspace

import (
	"strings"
	"testing"

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

func TestSettingsMenu(t *testing.T) {
	i18n.SetLocale("en")
	w, _, _ := testWS(t)
	m := w.SettingsMenu("%1", "c1")
	var names []string
	for _, it := range m.Items {
		names = append(names, it.Key+" "+it.Name)
	}
	got := strings.Join(names, "|")
	want := "u Update i-mux… (dev)| |a AI completion in the editor: Off|n Notifications: On|d Notification sound: On|i Icons: Auto|m Commit messages by: Auto (the pane's agent)|" +
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
		"m": "__settings menu commit-agent --pane %1 --client c1",
		"w": "__settings set on_arrival continue",
		"n": "__settings set notify off",
		"d": "__settings set notify_sound off",
		"l": "__settings set on_limit auto",
		"o": "__settings edit",
	} {
		if !strings.Contains(cmd(key), want) {
			t.Errorf("%s: %q lacks %q", key, cmd(key), want)
		}
	}
	if !strings.HasPrefix(cmd("a"), "display-popup -E") || !strings.HasPrefix(cmd("o"), "display-popup -E") {
		t.Error("setup and editing run in popups")
	}

	if c := cmd("u"); !strings.HasPrefix(c, "display-popup -E") || !strings.Contains(c, "update --popup") {
		t.Errorf("update row %q", c)
	}
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
	if c := m.Items[2].Cmd; !strings.Contains(c, "__settings set completion off") { // u, sep, a
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
