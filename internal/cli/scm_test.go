package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
)

// TestSCMAsks (T63.3, T70.2): a prompt's label is translated and safe in
// command-prompt's -p (comma-separated, a format: no "," "#" or "%"), and
// its answer goes to one place. Every commit but an amend (which keeps the
// last message) can ask its message in the commit box, and the box knows
// no other.
func TestSCMAsks(t *testing.T) {
	defer i18n.SetLocale("en")
	for op, a := range asks {
		_, inBox := sidebar.CommitAsks[op]
		amend := slices.Contains(a.git, "--amend")
		if commit := len(a.git) > 0 && a.git[0] == "commit"; commit && !amend != inBox || !commit && inBox {
			t.Errorf("%s: in the box's asks %v", op, inBox)
		}
	}
	for op, key := range sidebar.CommitAsks {
		if _, ok := asks[op]; !ok || !i18n.Has(key) {
			t.Errorf("the box asks for %s (%s): no such commit or text", op, key)
		}
	}
	for _, loc := range []string{"en", "tr"} {
		i18n.SetLocale(loc)
		for op, a := range asks {
			if slices.Contains(a.git, "--amend") {
				continue // never asked: an empty box keeps the last message
			}
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
