package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
)

// TestSCMAsks (T63.3): a prompt's label is translated and safe in
// command-prompt's -p (comma-separated, a format: no "," "#" or "%"), and
// its answer goes to one place.
func TestSCMAsks(t *testing.T) {
	defer i18n.SetLocale("en")
	for _, loc := range []string{"en", "tr"} {
		i18n.SetLocale(loc)
		for op, a := range asks {
			if l := i18n.T(a.label); l == a.label || strings.ContainsAny(l, ",#%") {
				t.Errorf("%s %s: label %q", loc, op, l)
			}
			n := 0
			for _, set := range []bool{len(a.git) > 0, a.next != "", a.popup != ""} {
				if set {
					n++
				}
			}
			if n != 1 {
				t.Errorf("%s: its answer goes to %d places", op, n)
			}
			if _, ok := asks[a.next]; a.next != "" && !ok {
				t.Errorf("%s: no next step %q", op, a.next)
			}
			if a.popup != "" && !gitPopupOps[a.popup] {
				t.Errorf("%s: no popup %q", op, a.popup)
			}
		}
	}
}

func TestCloneDest(t *testing.T) {
	dir := t.TempDir()
	for url, want := range map[string]string{
		"https://github.com/o/repo.git": "repo", "git@github.com:o/tool": "tool", "https://h/o/site/": "site",
		"/srv/git/lib.git": "lib", `C:\src\app`: "app", "..": "repo", "": "repo",
	} {
		if got := cloneDest(dir, url); got != filepath.Join(dir, want) {
			t.Errorf("cloneDest(%q) = %q, want %q", url, got, want)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "repo-2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := cloneDest(dir, "https://github.com/o/repo.git"); got != filepath.Join(dir, "repo-3") {
		t.Errorf("taken name: %q", got)
	}
}
