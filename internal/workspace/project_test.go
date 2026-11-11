package workspace

import (
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
)

// The "which agent?" menus end with a plain terminal.
func TestMenusOfferTerminal(t *testing.T) {
	i18n.SetLocale("en")
	w, _, _ := testWS(t)
	p := &Pane{ID: "%3", CWD: "/x"}
	for name, m := range map[string]*Menu{"split": w.SplitMenu(p, "/dev/ttys1", true), "tab": w.TabMenu(p, "/dev/ttys1")} {
		last := m.Items[len(m.Items)-1]
		if last.Name != "Terminal (shell)" || last.Key != "t" || !strings.Contains(last.Cmd, " shell ") {
			t.Errorf("%s menu ends with %+v", name, last)
		}
	}
	if c := w.SplitMenu(p, "", true).Items; !strings.Contains(c[len(c)-1].Cmd, "split shell -h") {
		t.Errorf("split shell cmd %q", c[len(c)-1].Cmd)
	}
}

// The "+" of Agents asks which agent, then where: right, left, down, up
// (split-window -b for left and up) or a new tab.
func TestNewAgentAndPlaceMenus(t *testing.T) {
	i18n.SetLocale("en")
	w, _, _ := testWS(t)
	p := &Pane{ID: "%3", CWD: "/x"}
	m := w.NewAgentMenu(p, "/dev/ttys1")
	if m.Title != "New agent with:" || !strings.Contains(m.Items[0].Cmd, "__menu --place claude --pane %3 --client /dev/ttys1") {
		t.Fatalf("new agent menu %q %+v", m.Title, m.Items[0])
	}
	if c := cmdOf(m, "t"); !strings.Contains(c, "__menu --place shell") {
		t.Errorf("terminal: %q", c)
	}
	// Then a web page (D47): its prompt; the answer opens in the browser.
	if last := m.Items[len(m.Items)-1]; last.Key != "w" || !strings.Contains(last.Cmd, "set-option -g @imux_web_pane %3") ||
		!strings.Contains(last.Cmd, "-I http://localhost:3000") || !strings.Contains(last.Cmd, "web open --from-option") {
		t.Errorf("new agent menu ends with %+v", last)
	}
	m = w.PlaceMenu(p, "", "codex")
	if m.Title != "Codex: open where?" {
		t.Errorf("title %q", m.Title)
	}
	want := map[string]string{"r": "split codex -h --pane", "l": "split codex -h --before --pane", "d": "split codex -v --pane", "u": "split codex -v --before --pane", "t": "tab codex --pane"}
	for _, it := range m.Items {
		if it.Key == "" {
			continue
		}
		if !strings.Contains(it.Cmd, want[it.Key]) {
			t.Errorf("%s: %q", it.Key, it.Cmd)
		}
		delete(want, it.Key)
	}
	if len(want) != 0 {
		t.Errorf("missing places %v", want)
	}
	// The project's "No agents" pane: the agent starts in its place.
	empty := &Pane{ID: "%4", CWD: "/x", Role: sidebar.RoleEmpty}
	if m := w.NewAgentMenu(empty, ""); !strings.Contains(m.Items[0].Cmd, "split claude -h") {
		t.Errorf("empty pane: %+v", m.Items[0])
	}
}
