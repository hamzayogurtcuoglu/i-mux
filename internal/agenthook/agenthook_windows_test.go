//go:build windows

package agenthook

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Claude's hooks on Windows use its exec form, with no shell; its status line, which has only a
// shell form, gets imux's path with / and no space (§7.9).
func TestClaudeOnWindows(t *testing.T) {
	h := ClaudeHook(`C:\Program Files\imux\imux.exe`)
	if h["command"] != `C:\Program Files\imux\imux.exe` || !slices.Equal(h["args"].([]string), []string{"__hook", "claude"}) {
		t.Errorf("hook %v", h)
	}
	if got := ClaudeStatusLine(`C:\Users\a\.local\bin\imux.exe`); got != "C:/Users/a/.local/bin/imux.exe __statusline claude" {
		t.Errorf("status line %q", got)
	}
	dir := filepath.Join(t.TempDir(), "with space")
	self := filepath.Join(dir, "imux.exe")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(self, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	got := ClaudeStatusLine(self)
	if prog, _, _ := strings.Cut(got, " __statusline"); strings.ContainsAny(prog, ` \`) || !strings.HasSuffix(got, " __statusline claude") {
		t.Errorf("status line with a space in the path: %q", got)
	}
}
