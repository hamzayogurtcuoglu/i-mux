package folders

import (
	"os"
	"path/filepath"
	"testing"
)

// The folder picker takes Windows' paths: a drive, "~\", a trailing "\"
// (§7.8).
func TestBrowseWindowsPaths(t *testing.T) {
	home := t.TempDir()
	for _, d := range []string{`code\alpha`, `code\beta`, "docs"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{`C:\Users`, `~\code`, "~"} {
		if !IsPath(q) {
			t.Errorf("IsPath(%q) = false", q)
		}
	}
	if got := Expand(`~\code`, home); got != home+`\code` {
		t.Errorf("Expand = %q", got)
	}
	got := Browse(`~\code\a`, home)
	if len(got) != 1 || got[0].Path != filepath.Join(home, "code", "alpha") {
		t.Fatalf("Browse(~\\code\\a) = %+v", got)
	}
	if c := Complete(`~\code\a`, home, got[0]); c != `~\code\alpha\` {
		t.Errorf("Complete = %q", c)
	}
	if got := Browse(home+`\code\`, home); len(got) != 2 {
		t.Errorf("Browse(<home>\\code\\) = %+v", got)
	}
}
