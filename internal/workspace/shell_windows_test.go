//go:build windows

package workspace

import "testing"

// Claude Code runs its status line with Git Bash on Windows: a double-quoted
// path with forward slashes, bash's specials escaped.
func TestStatuslineCmdWindows(t *testing.T) {
	for _, c := range []struct{ self, want string }{
		{`C:\Users\me\AppData\Local\imux\bin\imux.exe`, `"C:/Users/me/AppData/Local/imux/bin/imux.exe" __statusline claude`},
		{`C:\Users\Ada Lovelace\bin\imux.exe`, `"C:/Users/Ada Lovelace/bin/imux.exe" __statusline claude`},
		{`D:\$x\a` + "`" + `b\imux.exe`, `"D:/\$x/a` + "\\`" + `b/imux.exe" __statusline claude`},
	} {
		if got := statuslineCmd(c.self); got != c.want {
			t.Errorf("%s: %s, want %s", c.self, got, c.want)
		}
	}
}

// The preview popup pages through imux __page, not a PowerShell pipe.
func TestPagedCmdWindows(t *testing.T) {
	got := pagedCmd([]string{`C:\imux\imux.exe`, "handoff", "--pane", "%3", "--print"})
	if want := `& C:/imux/imux.exe __page handoff --pane %3 --print`; got != want {
		t.Fatalf("%s, want %s", got, want)
	}
}
