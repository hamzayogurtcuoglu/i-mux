package workspace

import (
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
)

// TestKeysMenu (T71.2): the help names every agent's key, the sidebar's
// five views, and Ctrl+V / Alt+V while pasting images is on.
func TestKeysMenu(t *testing.T) {
	i18n.SetLocale("en")
	cfg := config.Default()
	w := &WS{Cfg: cfg}
	text := func() string {
		var names []string
		for _, it := range w.KeysMenu().Items {
			names = append(names, it.Name)
		}
		return strings.Join(names, "\n")
	}
	got := text()
	for _, want := range []string{"c a p x e", "4 git", "5 jobs", "Ctrl+V"} {
		if !strings.Contains(got, want) {
			t.Errorf("lacks %q:\n%s", want, got)
		}
	}
	cfg.Paste.Images = false
	if strings.Contains(text(), "Ctrl+V") {
		t.Error("Ctrl+V listed while it is not bound")
	}
}
