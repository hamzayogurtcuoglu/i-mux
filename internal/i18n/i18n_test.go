package i18n

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	cases := []struct{ cfg, ajan, lang, want string }{
		{"", "", "", "en"},
		{"en", "tr", "tr_TR.UTF-8", "en"},
		{"", "", "en_US.UTF-8", "en"},
		{"", "", "C", "en"},
		{"", "xx", "en_US", "en"},
		{"", "", "de_DE.UTF-8", "en"},
	}
	for _, c := range cases {
		if got := Resolve(c.cfg, c.ajan, c.lang); got != c.want {
			t.Errorf("Resolve(%q,%q,%q) = %q, want %q", c.cfg, c.ajan, c.lang, got, c.want)
		}
	}
}

func TestT(t *testing.T) {
	SetLocale("en")
	if got := T("cli.unknown_command", "x"); got != `unknown command "x"` {
		t.Fatalf("got %q", got)
	}
	if got := T("no.such.key"); got != "no.such.key" {
		t.Fatalf("missing key should return the key, got %q", got)
	}
}

// TestCatalogCoversCode fails if any translation call (i18n dot T) in the module uses a
// key that is missing from the English catalog.
func TestCatalogCoversCode(t *testing.T) {
	root := moduleRoot(t)
	re := regexp.MustCompile(`i18n\.T\(\s*"([^"]+)"`)
	var missing []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "testdata" || name == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range re.FindAllSubmatch(b, -1) {
			if !Has(string(m[1])) {
				rel, _ := filepath.Rel(root, p)
				missing = append(missing, rel+": "+string(m[1]))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) > 0 {
		t.Fatalf("keys missing from the en catalog:\n%s", strings.Join(missing, "\n"))
	}
}

// TestLocalesCoverEnglish enforces full key coverage for every non-English catalog.
func TestLocalesCoverEnglish(t *testing.T) {
	for _, l := range Locales() {
		if l == "en" {
			continue
		}
		cat := Catalog(l)
		for _, k := range Keys() {
			if _, ok := cat[k]; !ok {
				t.Errorf("locale %s is missing key %s", l, k)
			}
		}
		for k := range cat {
			if !Has(k) {
				t.Errorf("locale %s has key %s that en does not have", l, k)
			}
		}
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
