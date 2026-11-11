//go:build !windows

package tmux

import "testing"

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
