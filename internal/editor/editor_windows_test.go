//go:build windows

package editor

import (
	"strings"
	"testing"
)

func TestChooseWindows(t *testing.T) {
	have, git := fakeTools(t)
	if Choose("") != nil {
		t.Fatal("no editor at all")
	}
	// Git for Windows' vim and nano are found though they are not on the
	// PATH; nano before vim, as on Unix.
	git["vim"] = true
	if got := strings.Join(Choose(""), " "); got != `C:\Git\usr\bin\vim.exe` {
		t.Fatalf("git vim %q", got)
	}
	git["nano"] = true
	if got := strings.Join(Choose(""), " "); got != `C:\Git\usr\bin\nano.exe` {
		t.Fatalf("git nano %q", got)
	}
	have["edit"] = true // Microsoft Edit, in Windows 11
	if got := strings.Join(Choose(""), " "); got != "/bin/edit" {
		t.Fatalf("edit before Git's editors %q", got)
	}
	have["nvim"] = true
	if got := strings.Join(Choose(""), " "); got != "/bin/nvim" {
		t.Fatalf("nvim %q", got)
	}
	t.Setenv("EDITOR", "notepad")
	have["notepad"] = true
	if got := strings.Join(Choose(""), " "); got != "/bin/notepad" {
		t.Fatalf("$EDITOR %q", got)
	}
	have["micro"] = true
	if got := strings.Join(Choose(""), " "); got != "/bin/micro" {
		t.Fatalf("micro first %q", got)
	}
}

func TestCommandWindows(t *testing.T) {
	for _, c := range []struct {
		ed   []string
		line int
		want string
	}{
		{[]string{`C:\Program Files\Git\usr\bin\vim.exe`}, 3, `C:\Program Files\Git\usr\bin\vim.exe +set mouse=a +3 a.go`},
		{[]string{`C:\nvim\bin\NVIM.EXE`}, 0, `C:\nvim\bin\NVIM.EXE +set mouse=a a.go`},
		{[]string{`C:\Windows\System32\edit.exe`}, 9, `C:\Windows\System32\edit.exe a.go:9`},
		{[]string{`C:\Windows\System32\edit.exe`}, 0, `C:\Windows\System32\edit.exe a.go`},
		{[]string{`C:\VS Code\bin\code.cmd`}, 7, `C:\VS Code\bin\code.cmd -g a.go:7`},
		{[]string{`C:\micro\micro.exe`}, 12, `C:\micro\micro.exe +12 a.go`},
	} {
		if got := strings.Join(Command(c.ed, "a.go", c.line), " "); got != c.want {
			t.Errorf("%v: %q, want %q", c.ed, got, c.want)
		}
	}
}
