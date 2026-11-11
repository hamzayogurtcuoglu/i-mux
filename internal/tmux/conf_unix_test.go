//go:build !windows

package tmux

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func golden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/tmux -update)", err)
	}
	if string(want) != got {
		t.Fatalf("%s differs from golden file; run with -update to accept.\n--- got ---\n%s", name, got)
	}
}

// The golden files are tmux's config, with sh command lines; on Windows the
// config is psmux's (see conf_windows_test.go).
func TestGenerateConfGolden(t *testing.T) {
	golden(t, "tmux-3.4.conf.golden", GenerateConf(confInput("3.4")))
	golden(t, "tmux-3.2.conf.golden", GenerateConf(confInput("3.2a")))
}

// A single quote inside a word: tmux and sh each join quoted pieces.
func TestQuoteSingleQuote(t *testing.T) {
	if got, want := Quote("it's"), `'it'\''s'`; got != want {
		t.Errorf("Quote(%q) = %q, want %q", "it's", got, want)
	}
	if got := ShJoin("/a b/imux", "__menu", "it's"); got != `'/a b/imux' __menu 'it'"'"'s'` {
		t.Fatalf("ShJoin: %s", got)
	}
}
