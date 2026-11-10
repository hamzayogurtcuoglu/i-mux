package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/nerdfont"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestFontInstallAsks (M43): install changes nothing without an answer:
// not from a pipe, and not from /dev/null (a character device whose read
// ends at once); --check fails while the font is not in place.
func TestFontInstallAsks(t *testing.T) {
	i18n.SetLocale("en")
	testutil.Isolate(t)
	home := t.TempDir()
	oldOS, oldWSL, oldHome, oldFetch, oldSys := nerdfont.GOOS, nerdfont.IsWSL, nerdfont.Home, nerdfont.Fetch, nerdfont.SystemFonts["linux"]
	defer func() {
		nerdfont.GOOS, nerdfont.IsWSL, nerdfont.Home, nerdfont.Fetch, nerdfont.SystemFonts["linux"] = oldOS, oldWSL, oldHome, oldFetch, oldSys
	}()
	nerdfont.GOOS, nerdfont.IsWSL, nerdfont.Home = "linux", func() bool { return false }, func() string { return home }
	nerdfont.SystemFonts["linux"] = filepath.Join(home, "system-fonts")
	fetched := false
	nerdfont.Fetch = func(context.Context, string, string) error { fetched = true; return os.ErrPermission }
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	var out bytes.Buffer
	if code := Run(&Env{Stdin: null, Stdout: &out, Stderr: &out}, []string{"font", "install"}); code != ExitOK {
		t.Fatalf("exit %d\n%s", code, out.String())
	}
	if fetched || !strings.Contains(out.String(), "Go ahead? [Y/n]") || !strings.Contains(out.String(), "Nothing changed.") {
		t.Fatalf("no answer installed it (fetched %v):\n%s", fetched, out.String())
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	defer r.Close()
	out.Reset()
	Run(&Env{Stdin: r, Stdout: &out, Stderr: &out}, []string{"font", "install"})
	if fetched || !strings.Contains(out.String(), "or add --yes") {
		t.Fatalf("a pipe installed it:\n%s", out.String())
	}
	if code := Run(&Env{Stdin: null, Stdout: &out, Stderr: &out}, []string{"font", "--check"}); code != ExitError {
		t.Fatalf("--check without the font: exit %d", code)
	}
}
