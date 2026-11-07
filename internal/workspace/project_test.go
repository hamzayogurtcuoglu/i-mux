package workspace

import (
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
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
