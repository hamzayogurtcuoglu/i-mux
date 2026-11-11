//go:build !windows

package editor

import (
	"strings"
	"testing"
)

func TestChoose(t *testing.T) {
	have, git := fakeTools(t)
	git["nano"] = true // Git for Windows' programs count only on Windows
	have["vim"] = true
	if got := strings.Join(Choose(""), " "); got != "/bin/vim" {
		t.Fatalf("vim %q", got)
	}
	have["nano"] = true
	if got := strings.Join(Choose(""), " "); got != "/bin/nano" {
		t.Fatalf("nano before vim %q", got)
	}
	t.Setenv("EDITOR", "hx --flag")
	have["hx"] = true
	if got := strings.Join(Choose(""), " "); got != "/bin/hx --flag" {
		t.Fatalf("$EDITOR %q", got)
	}
	have["micro"] = true
	if got := strings.Join(Choose(""), " "); got != "/bin/micro" {
		t.Fatalf("micro first %q", got)
	}
	have["myed"] = true
	if got := strings.Join(Choose("myed -x"), " "); got != "/bin/myed -x" {
		t.Fatalf("configured %q", got)
	}
	// "edit" is nothing special here (Debian's is run-mailcap).
	if got := strings.Join(Command([]string{"/usr/bin/edit"}, "a.go", 4), " "); got != "/usr/bin/edit +4 a.go" {
		t.Fatalf("edit %q", got)
	}
}
