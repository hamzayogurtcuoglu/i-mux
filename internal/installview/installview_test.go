package installview

import (
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
)

func screen(m *Model) string {
	var b strings.Builder
	for _, l := range m.Render(60, 12) {
		b.WriteString(sidebar.Plain(l) + "\n")
	}
	return b.String()
}

func TestInstallPopup(t *testing.T) {
	i18n.SetLocale("en")
	m := &Model{Icon: "◎", Name: "Codex", Cmd: "npm i -g @openai/codex"}
	s := screen(m)
	for _, want := range []string{"◎ Codex is not installed", "To continue this chat in Codex", "$ npm i -g @openai/codex", " Install now ", "[ Close ]", "Enter install"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in\n%s", want, s)
		}
	}
	if r, done := m.Key("enter"); !done || r != Install {
		t.Fatalf("Enter: %v %v", r, done)
	}
	if _, done := m.Key("tab"); done {
		t.Fatal("Tab chose")
	}
	if r, done := m.Key("enter"); !done || r != Close {
		t.Fatalf("Tab, Enter: %v %v", r, done)
	}
	if r, done := m.Key("esc"); !done || r != Close {
		t.Fatalf("Esc: %v %v", r, done)
	}
	// A click on a button chooses it; elsewhere nothing.
	m = &Model{Icon: "◎", Name: "Codex", Cmd: "npm i -g @openai/codex"}
	lines := m.Render(60, 12)
	if _, done := m.Click(5, 1); done {
		t.Fatal("a click on the title chose")
	}
	y := m.btnY
	row := sidebar.Plain(lines[y-1])
	x := sidebar.DisplayWidth(row[:strings.Index(row, "Close")]) + 1
	if r, done := m.Click(x, y); !done || r != Close {
		t.Fatalf("click on Close: %v %v (row %q)", r, done, row)
	}
}

// Without the program the command needs (npm), the popup says how to get it
// and only closes.
func TestInstallPopupMissingTool(t *testing.T) {
	i18n.SetLocale("en")
	m := &Model{Icon: "◎", Name: "Codex", Cmd: "npm i -g @openai/codex", Missing: "npm"}
	s := screen(m)
	if !strings.Contains(s, "Codex installs with npm") || !strings.Contains(s, "nodejs.org") || strings.Contains(s, "Install now") {
		t.Fatalf("popup:\n%s", s)
	}
	if r, done := m.Key("enter"); !done || r != Close {
		t.Fatalf("Enter: %v %v", r, done)
	}
	if _, done := m.Key("i"); done {
		t.Fatal("i installed without npm")
	}
}
