// Package editor picks the editor a file opens in and builds its command
// (ARCHITECTURE D27, D45): the configured one, else micro (the PATH's, or
// the one i-mux got), $VISUAL, $EDITOR, nano, vim (with the mouse on), vi.
// On Windows Neovim and Microsoft Edit join them, and Git for Windows' nano
// and vim count as installed.
package editor

import (
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/hamzayogurtcuoglu/i-mux/internal/winx"
)

// LookPath is exec.LookPath; tests replace it.
var LookPath = defaultLookPath

var defaultLookPath = exec.LookPath

// GitTool finds a program of Git for Windows ("" elsewhere); tests replace
// it.
var GitTool = defaultGitTool

var defaultGitTool = winx.GitTool

// find is the path of program name: on the PATH, else (Windows) in Git.
func find(name string) (string, bool) {
	if p, err := LookPath(name); err == nil {
		return p, true
	}
	if runtime.GOOS == "windows" {
		if p := GitTool(name); p != "" {
			return p, true
		}
	}
	return "", false
}

// Choose returns the editor command (program and its own arguments): the
// configured one, then micro (the PATH's, then i-mux's), $VISUAL, $EDITOR,
// nano, vim, vi (on Windows: nvim, edit, nano, vim).
func Choose(configured string) []string {
	try := func(cmd string) []string {
		f := strings.Fields(cmd)
		if len(f) == 0 {
			return nil
		}
		if p, ok := find(f[0]); ok {
			return append([]string{p}, f[1:]...)
		}
		return nil
	}
	for i, c := range candidates(configured) {
		if e := try(c); e != nil {
			return e
		}
		if i == 1 {
			if m := OurMicro(); m != "" {
				return []string{m}
			}
		}
	}
	return nil
}

// Chosen reports whether the user chose an editor: the config's, $VISUAL
// or $EDITOR.
func Chosen(configured string) bool {
	return strings.TrimSpace(configured+os.Getenv("VISUAL")+os.Getenv("EDITOR")) != ""
}

// Command is the argv that opens path at line (0: the top) in editor.
func Command(editor []string, path string, line int) []string {
	if len(editor) == 0 {
		return nil
	}
	name := progName(editor[0])
	argv := append([]string{}, editor...)
	if a, ok := platformCommand(name, argv, path, line); ok {
		return a
	}
	switch name {
	case "code", "code-insiders", "cursor":
		target := path
		if line > 0 {
			target += ":" + strconv.Itoa(line)
		}
		return append(argv, "-g", target)
	case "vim", "vi", "nvim":
		argv = append(argv, "+set mouse=a")
	case "micro":
		// A scroll bar (D45): a block in reverse video, as nano's.
		argv = append(argv, "-scrollbar", "true")
		if editor[0] == OurMicro() {
			argv = append(argv, "-scrollbarchar", " ")
		}
	case "nano":
		// GNU nano only: macOS's nano may be pico, which has neither.
		if v := NanoVersion(editor[0]); v > 0 {
			argv = append(argv, "--mouse")
			if v >= 5 {
				argv = append(argv, "--indicator") // where you are, at the right edge
			}
		}
	}
	if line > 0 {
		argv = append(argv, "+"+strconv.Itoa(line))
	}
	return append(argv, path)
}

// VSCode returns the VS Code command line tool when VS Code is installed
// ("" otherwise): `code` on the PATH, else the one inside the macOS app (on
// Windows: in VS Code's install folder).
func VSCode() string {
	if p, err := LookPath("code"); err == nil {
		return p
	}
	for _, p := range vscodePaths() {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

var (
	nanoMu   sync.Mutex
	nanoSeen = map[string]int{}
	nanoRe   = regexp.MustCompile(`GNU nano,? version (\d+)\.`)
)

// NanoVersion is the major version of the GNU nano at path (0 when it is
// not GNU nano, or unknown), asked once.
var NanoVersion = func(path string) int {
	nanoMu.Lock()
	defer nanoMu.Unlock()
	if v, ok := nanoSeen[path]; ok {
		return v
	}
	out, _ := exec.Command(path, "--version").Output()
	v := 0
	if m := nanoRe.FindSubmatch(out); m != nil {
		v, _ = strconv.Atoi(string(m[1]))
	}
	nanoSeen[path] = v
	return v
}
