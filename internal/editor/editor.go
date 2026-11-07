// Package editor picks the editor a file opens in and builds its command
// (ARCHITECTURE D27): the configured one, else micro, $VISUAL, $EDITOR,
// nano, vim (with the mouse on), vi.
package editor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// LookPath is exec.LookPath; tests replace it.
var LookPath = defaultLookPath

var defaultLookPath = exec.LookPath

// Choose returns the editor command (program and its own arguments): the
// configured one, then micro, $VISUAL, $EDITOR, nano, vim, vi.
func Choose(configured string) []string {
	try := func(cmd string) []string {
		f := strings.Fields(cmd)
		if len(f) == 0 {
			return nil
		}
		if p, err := LookPath(f[0]); err == nil {
			return append([]string{p}, f[1:]...)
		}
		return nil
	}
	for _, c := range []string{configured, "micro", os.Getenv("VISUAL"), os.Getenv("EDITOR"), "nano", "vim", "vi"} {
		if e := try(c); e != nil {
			return e
		}
	}
	return nil
}

// Command is the argv that opens path at line (0: the top) in editor.
func Command(editor []string, path string, line int) []string {
	if len(editor) == 0 {
		return nil
	}
	name := filepath.Base(editor[0])
	argv := append([]string{}, editor...)
	switch name {
	case "code", "code-insiders", "cursor":
		target := path
		if line > 0 {
			target += ":" + strconv.Itoa(line)
		}
		return append(argv, "-g", target)
	case "vim", "vi", "nvim":
		argv = append(argv, "+set mouse=a")
	}
	if line > 0 {
		argv = append(argv, "+"+strconv.Itoa(line))
	}
	return append(argv, path)
}

// VSCode returns the VS Code command line tool when VS Code is installed
// ("" otherwise): `code` on the PATH, else the one inside the macOS app.
func VSCode() string {
	if p, err := LookPath("code"); err == nil {
		return p
	}
	for _, p := range []string{
		"/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code",
		filepath.Join(os.Getenv("HOME"), "Applications/Visual Studio Code.app/Contents/Resources/app/bin/code"),
	} {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}
