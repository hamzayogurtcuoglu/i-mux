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
	for name, m := range map[string]*Menu{"split": w.SplitMenu(p, "/dev/ttys1", true, false, ""), "tab": w.TabMenu(p, "/dev/ttys1", "")} {
		last := m.Items[len(m.Items)-1]
		if last.Name != "Terminal (shell)" || last.Key != "t" || !strings.Contains(last.Cmd, " shell ") {
			t.Errorf("%s menu ends with %+v", name, last)
		}
	}
	if c := w.SplitMenu(p, "", true, false, "").Items; !strings.Contains(c[len(c)-1].Cmd, "split shell -h") {
		t.Errorf("split shell cmd %q", c[len(c)-1].Cmd)
	}
}

// The "+" of Agents asks which agent, then where: right, left, down, up
// (split-window -b for left and up) or a new tab.
func TestNewAgentAndPlaceMenus(t *testing.T) {
	i18n.SetLocale("en")
	w, _, _ := testWS(t)
	p := &Pane{ID: "%3", CWD: "/x"}
	m := w.NewAgentMenu(p, "/dev/ttys1", "")
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
	m = w.PlaceMenu(p, "", "codex", "")
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
	if m := w.NewAgentMenu(empty, "", ""); !strings.Contains(m.Items[0].Cmd, "split claude -h") {
		t.Errorf("empty pane: %+v", m.Items[0])
	}
}

// The menus that start a new agent list the environments (D101): With
// <name>… opens the menu again for it, whose choices pass --env on and
// which offers Without <name>…; the New agent menu lists none, and for an
// environment (the Env view's New agent with it…) has no Web page….
func TestMenusOfferEnvironments(t *testing.T) {
	i18n.SetLocale("en")
	w, _, _ := testWS(t)
	p := &Pane{ID: "%3", CWD: "/x"}
	names := func(m *Menu) string {
		var b strings.Builder
		for _, it := range m.Items {
			b.WriteString(it.Name + "\n")
		}
		return b.String()
	}
	if m := w.PlaceMenu(p, "", "claude", ""); strings.Contains(names(m), "🔑") {
		t.Fatalf("no environments, yet:\n%s", names(m))
	}
	writeEnv(t, "glm", "TOKEN=x\n")
	writeEnv(t, "proxy", "HTTPS_PROXY=http://proxy.test:8080\n")

	m := w.PlaceMenu(p, "c1", "claude", "")
	n := len(m.Items)
	if it := m.Items[n-2]; it.Name != "🔑 With glm…" || it.Key != "1" || !strings.Contains(it.Cmd, "__menu --place claude --env glm --pane %3 --client c1") {
		t.Errorf("glm: %+v", it)
	}
	if it := m.Items[n-1]; it.Name != "🔑 With proxy…" || it.Key != "2" || !strings.Contains(it.Cmd, "__menu --place claude --env proxy --pane %3") {
		t.Errorf("proxy: %+v", it)
	}
	if c := cmdOf(m, "r"); !strings.Contains(c, "split claude -h --pane %3") {
		t.Errorf("right, without an environment: %q", c)
	}

	m = w.PlaceMenu(p, "c1", "claude", "glm")
	if m.Title != "Claude · 🔑 glm: open where?" || strings.Contains(names(m), "With glm") || !strings.Contains(names(m), "🔑 Without glm…") {
		t.Errorf("for glm: %q\n%s", m.Title, names(m))
	}
	for key, want := range map[string]string{
		"r": "split claude -h --env glm --pane %3 --client c1", "l": "split claude -h --before --env glm --pane %3",
		"d": "split claude -v --env glm --pane %3", "u": "split claude -v --before --env glm --pane %3",
		"t": "tab claude --env glm --pane %3", "w": "worktree new --from-option --env glm",
		"0": "__menu --place claude --pane %3 --client c1", "2": "__menu --place claude --env proxy --pane %3",
	} {
		if c := cmdOf(m, key); !strings.Contains(c, want) {
			t.Errorf("%s: %q, want %q", key, c, want)
		}
	}
	if c := cmdOf(w.PlaceMenu(p, "", ShellAgent, "glm"), "d"); !strings.Contains(c, "split shell -v --env glm --pane %3") {
		t.Errorf("a terminal with glm: %q", c)
	}

	// The split and New tab menus list them too.
	m = w.SplitMenu(p, "c1", true, true, "glm")
	if m.Title != "🔑 glm · Split left: new pane with…" || !strings.Contains(cmdOf(m, "c"), "split claude -h --before --env glm --pane %3") ||
		!strings.Contains(cmdOf(m, "t"), "split shell -h --before --env glm --pane %3") ||
		!strings.Contains(cmdOf(m, "0"), "__menu --split h --before --pane %3") || !strings.Contains(cmdOf(m, "2"), "__menu --split h --before --env proxy --pane %3") {
		t.Errorf("split menu %q:\n%+v", m.Title, m.Items)
	}
	m = w.TabMenu(p, "c1", "")
	if !strings.Contains(cmdOf(m, "c"), "tab claude --pane %3") || !strings.Contains(cmdOf(m, "1"), "__menu --tab --env glm --pane %3") {
		t.Errorf("tab menu %+v", m.Items)
	}
	if c := cmdOf(w.TabMenu(p, "c1", "proxy"), "t"); !strings.Contains(c, "tab shell --env proxy --pane %3") {
		t.Errorf("a terminal tab with proxy: %q", c)
	}

	// The New agent menu lists none; for an environment its choices carry it.
	if m := w.NewAgentMenu(p, "c1", ""); strings.Contains(names(m), "🔑") {
		t.Errorf("new agent menu:\n%s", names(m))
	}
	m = w.NewAgentMenu(p, "c1", "glm")
	if m.Title != "🔑 glm · New agent with:" || !strings.Contains(cmdOf(m, "c"), "__menu --place claude --env glm --pane %3") ||
		!strings.Contains(cmdOf(m, "t"), "__menu --place shell --env glm --pane %3") || cmdOf(m, "w") != "" {
		t.Errorf("new agent with glm %q:\n%+v", m.Title, m.Items)
	}
	empty := &Pane{ID: "%4", CWD: "/x", Role: sidebar.RoleEmpty}
	if c := cmdOf(w.NewAgentMenu(empty, "", "glm"), "c"); !strings.Contains(c, "split claude -h --env glm --pane %4") {
		t.Errorf("the No agents pane with glm: %q", c)
	}
}
