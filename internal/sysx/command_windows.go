//go:build windows

package sysx

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// Command is how the program at path runs with args, with no shell between:
// argv[0] is what is run. An npm shim (<name>.cmd) is read and what it runs
// is run, node.exe and the package's script or the package's .exe, so
// cmd.exe never parses the arguments (a prompt's %, ^, & and "; §7.9).
// Anything else runs as it is.
func Command(path string, args ...string) []string {
	if p := npmShim(path); p != nil {
		return append(p, args...)
	}
	return append([]string{path}, args...)
}

// npmShim is what an npm shim runs before its arguments: node and a script,
// or a program; nil when path is no shim i-mux can read. npm's cmd-shim ends
// the shim with the command line, before %*:
//
//	endLocal & goto #_undefined_# 2>NUL || title %COMSPEC% & "%_prog%"  "%dp0%\node_modules\pkg\cli.js" %*
//	"%dp0%\node_modules\pkg\bin\tool.exe"   %*
//
// where %_prog% is the node.exe beside the shim, else node on the PATH (older
// npm wrote "%~dp0\node.exe" "…" %* and node "…" %*).
func npmShim(path string) []string {
	if ext := strings.ToLower(filepath.Ext(path)); ext != ".cmd" && ext != ".bat" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 64<<10))
	if err != nil {
		return nil
	}
	dir := filepath.Dir(path)
	for _, line := range strings.Split(string(b), "\n") {
		head, _, ok := strings.Cut(strings.TrimSpace(line), "%*")
		if !ok {
			continue
		}
		head = strings.TrimPrefix(strings.TrimSpace(head), "@")
		var words []string
		if _, rest, ok := strings.Cut(head, `"%_prog%"`); ok {
			if words = cmdWords(rest, dir); words != nil {
				words = append([]string{"node"}, words...)
			}
		} else {
			words = cmdWords(head, dir)
		}
		if len(words) < 1 {
			continue
		}
		switch {
		case strings.EqualFold(words[0], "node"):
			words[0] = filepath.Join(dir, "node.exe")
			if !isFile(words[0]) {
				if words[0], err = exec.LookPath("node.exe"); err != nil {
					continue
				}
			}
		case !filepath.IsAbs(words[0]) || !isFile(words[0]):
			continue
		}
		return words
	}
	return nil
}

// cmdWords splits a cmd-shim command line into words: blanks apart, quotes
// taken away, %dp0% and %~dp0 the shim's folder. nil when a variable is left
// or the line holds more than words.
func cmdWords(s, dir string) []string {
	var words []string
	for s = strings.TrimSpace(s); s != ""; s = strings.TrimSpace(s) {
		var w string
		if s[0] == '"' {
			end := strings.IndexByte(s[1:], '"')
			if end < 0 {
				return nil
			}
			w, s = s[1:1+end], s[2+end:]
		} else {
			end := strings.IndexAny(s, " \t")
			if end < 0 {
				end = len(s)
			}
			w, s = s[:end], s[end:]
		}
		for _, v := range []string{"%dp0%", "%~dp0"} {
			if rest, ok := strings.CutPrefix(w, v); ok {
				w = filepath.Join(dir, rest)
			}
		}
		if strings.ContainsAny(w, "%&|<>^") {
			return nil
		}
		words = append(words, w)
	}
	return words
}

// InstallShell is how an install or fix command line runs: Windows
// PowerShell, there on every Windows, with no profile and any script
// allowed, as the agents' own install commands expect (§7.9).
func InstallShell(cmd string) []string {
	return []string{windowsPowerShell(), "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", cmd}
}

// UserShell is how a command line the user wrote for an agent (Claude's
// status line) runs, as Claude Code runs it on Windows: Git Bash's bash -c
// when Git for Windows is installed (CLAUDE_CODE_GIT_BASH_PATH first), else
// PowerShell.
func UserShell(cmd string) []string {
	if bash := gitBash(); bash != "" {
		return []string{bash, "-c", cmd}
	}
	return []string{windowsPowerShell(), "-NoProfile", "-Command", cmd}
}

// windowsPowerShell is powershell.exe (5.1), on every Windows.
func windowsPowerShell() string {
	return filepath.Join(systemRoot(), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
}

// gitBash is Git for Windows' bash.exe, "" without it: the one Claude Code
// is told about, else the one beside git on the PATH (<root>\cmd\git.exe or
// <root>\bin\git.exe gives <root>\bin\bash.exe).
func gitBash() string {
	if p := os.Getenv("CLAUDE_CODE_GIT_BASH_PATH"); p != "" && isFile(p) {
		return p
	}
	git, err := exec.LookPath("git.exe")
	if err != nil {
		return ""
	}
	if p := filepath.Join(filepath.Dir(filepath.Dir(git)), "bin", "bash.exe"); isFile(p) {
		return p
	}
	return ""
}

// ShortPath is p in its 8.3 form, which has no spaces where the volume
// keeps 8.3 names; p when it has none.
func ShortPath(p string) string {
	long, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return p
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetShortPathName(long, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 || int(n) > len(buf) {
		return p
	}
	return windows.UTF16ToString(buf[:n])
}
