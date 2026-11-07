package editor

import (
	"errors"
	"strings"
	"testing"
)

func TestChooseAndCommand(t *testing.T) {
	have := map[string]bool{}
	LookPath = func(n string) (string, error) {
		if have[n] {
			return "/bin/" + n, nil
		}
		return "", errors.New("no")
	}
	t.Cleanup(func() { LookPath = defaultLookPath })
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
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
	for _, c := range []struct {
		ed   []string
		line int
		want string
	}{
		{[]string{"/bin/micro"}, 12, "/bin/micro +12 a b.go"},
		{[]string{"/bin/vim"}, 3, "/bin/vim +set mouse=a +3 a b.go"},
		{[]string{"/bin/nano"}, 0, "/bin/nano a b.go"},
		{[]string{"/x/code"}, 7, "/x/code -g a b.go:7"},
	} {
		if got := strings.Join(Command(c.ed, "a b.go", c.line), " "); got != c.want {
			t.Errorf("%v: %q, want %q", c.ed, got, c.want)
		}
	}
	if Command(nil, "x", 1) != nil {
		t.Fatal("no editor")
	}
}
