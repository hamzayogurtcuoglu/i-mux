//go:build windows

package sysx

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// An npm shim runs as what it runs, so cmd.exe never parses the arguments (§7.9): node and the
// package's script, or the package's program; anything else runs as it is.
func TestCommandUnwrapsNpmShims(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	node := write("node.exe", "")
	tool := filepath.Join(dir, "node_modules", "pkg", "bin", "tool.exe")
	if err := os.MkdirAll(filepath.Dir(tool), 0o700); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join("node_modules", "pkg", "bin", "tool.exe"), "")
	script := write("cli.cmd", "@ECHO off\r\nGOTO start\r\n:find_dp0\r\nSET dp0=%~dp0\r\nEXIT /b\r\n:start\r\nSETLOCAL\r\nCALL :find_dp0\r\n\r\n"+
		"IF EXIST \"%dp0%\\node.exe\" (\r\n  SET \"_prog=%dp0%\\node.exe\"\r\n) ELSE (\r\n  SET \"_prog=node\"\r\n  SET PATHEXT=%PATHEXT:;.JS;=;%\r\n)\r\n\r\n"+
		"endLocal & goto #_undefined_# 2>NUL || title %COMSPEC% & \"%_prog%\"  \"%dp0%\\node_modules\\pkg\\cli.js\" %*\r\n")
	exe := write("tool.cmd", "@ECHO off\r\n\"%~dp0\\node_modules\\pkg\\bin\\tool.exe\"   %*\r\n")
	other := write("plain.exe", "")
	for _, c := range []struct {
		path string
		want []string
	}{
		{script, []string{node, filepath.Join(dir, "node_modules", "pkg", "cli.js"), "a 100%", `"q"`}},
		{exe, []string{tool, "a 100%", `"q"`}},
		{other, []string{other, "a 100%", `"q"`}},
	} {
		if got := Command(c.path, "a 100%", `"q"`); !slices.Equal(got, c.want) {
			t.Errorf("%s: %q, want %q", filepath.Base(c.path), got, c.want)
		}
	}
}

// ShortPath gives a path with no space where the volume keeps 8.3 names, and the path itself
// otherwise.
func TestShortPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "with space")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := ShortPath(dir); got == "" {
		t.Fatal("empty")
	}
	if got := ShortPath(filepath.Join(dir, "missing")); got != filepath.Join(dir, "missing") {
		t.Errorf("a missing path changed: %q", got)
	}
}
