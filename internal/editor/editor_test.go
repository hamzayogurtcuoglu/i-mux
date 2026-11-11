package editor

import (
	"errors"
	"strings"
	"testing"
)

// fakeTools makes LookPath find the programs in have (in /bin) and Git for
// Windows the ones in git; i-mux has no micro of its own.
func fakeTools(t *testing.T) (have, git map[string]bool) {
	t.Helper()
	have, git = map[string]bool{}, map[string]bool{}
	LookPath = func(n string) (string, error) {
		if have[n] {
			return "/bin/" + n, nil
		}
		return "", errors.New("no")
	}
	GitTool = func(n string) string {
		if git[n] {
			return `C:\Git\usr\bin\` + n + ".exe"
		}
		return ""
	}
	oldOurs := OurMicro
	OurMicro = func() string { return "" }
	t.Cleanup(func() { LookPath, GitTool, OurMicro = defaultLookPath, defaultGitTool, oldOurs })
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	return have, git
}

func TestChooseAndCommand(t *testing.T) {
	have := map[string]bool{}
	LookPath = func(n string) (string, error) {
		if have[n] {
			return "/bin/" + n, nil
		}
		return "", errors.New("no")
	}
	GitTool = func(string) string { return "" }
	ours := ""
	oldOurs, oldNano := OurMicro, NanoVersion
	OurMicro = func() string { return ours }
	nano := map[string]int{"/bin/nano": 6, "/old/nano": 2}
	NanoVersion = func(p string) int { return nano[p] }
	t.Cleanup(func() { LookPath, GitTool, OurMicro, NanoVersion = defaultLookPath, defaultGitTool, oldOurs, oldNano })
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	if Chosen("") {
		t.Fatal("chosen without a config, $VISUAL or $EDITOR")
	}
	have["vim"] = true
	if got := strings.Join(Choose(""), " "); got != "/bin/vim" {
		t.Fatalf("vim %q", got)
	}
	have["nano"] = true
	if got := strings.Join(Choose(""), " "); got != "/bin/nano" {
		t.Fatalf("nano before vim %q", got)
	}
	// The micro i-mux got comes before $EDITOR, as the PATH's does (D45).
	ours = "/home/u/.imux/tools/micro-2.0.15/micro"
	if got := strings.Join(Choose(""), " "); got != ours {
		t.Fatalf("i-mux's micro %q", got)
	}
	ours = ""
	t.Setenv("EDITOR", "hx --flag")
	have["hx"] = true
	if got := strings.Join(Choose(""), " "); got != "/bin/hx --flag" || !Chosen("") {
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
	ours = "/home/u/.imux/tools/micro-2.0.15/micro"
	for _, c := range []struct {
		ed   []string
		line int
		want string
	}{
		{[]string{"/bin/micro"}, 12, "/bin/micro -scrollbar true +12 a b.go"},
		{[]string{"/home/u/.imux/tools/micro-2.0.15/micro"}, 0, "/home/u/.imux/tools/micro-2.0.15/micro -scrollbar true -scrollbarchar   a b.go"},
		{[]string{"/bin/vim"}, 3, "/bin/vim +set mouse=a +3 a b.go"},
		{[]string{"/bin/nano"}, 0, "/bin/nano --mouse --indicator a b.go"},
		{[]string{"/old/nano"}, 4, "/old/nano --mouse +4 a b.go"}, // no --indicator before nano 5
		{[]string{"/usr/bin/nano"}, 0, "/usr/bin/nano a b.go"},    // pico (macOS): no flags
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

func TestNanoVersion(t *testing.T) {
	if NanoVersion("/no/such/nano") != 0 {
		t.Error("a missing nano has a version")
	}
	for out, want := range map[string]int{
		" GNU nano, version 6.2\n (C) 2022 the Free Software Foundation": 6,
		"GNU nano version 2.0.6 (compiled 10:06:28, Jun 27 2023)":        2,
		"Pico 5.09": 0,
	} {
		m := nanoRe.FindStringSubmatch(out)
		got := 0
		if m != nil {
			got = int(m[1][0] - '0')
		}
		if got != want {
			t.Errorf("%q: %d, want %d", out, got, want)
		}
	}
}
