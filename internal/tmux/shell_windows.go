//go:build windows

package tmux

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// On Windows psmux runs run-shell, if-shell and #() commands with PowerShell
// (pwsh, else Windows PowerShell), and imux sets default-shell to the same
// PowerShell, so every command line imux writes is PowerShell: a program is
// called with the call operator (& 'C:\path\imux.exe' args), words are single
// quoted ('' is a quote inside them), and output goes to $null.

// Silent is appended to every shell command imux's bindings, hooks and menu
// items run: tmux shows any output of run-shell (or a non-zero exit) in the
// pane's view mode, so imux reports results with display-message instead.
const Silent = " *>&1 | Out-Null; exit 0"

// HookTargetsActive reports whether hooks expand #{pane_id} as the active
// pane instead of the pane the event is about: psmux does.
const HookTargetsActive = true

// quoteJoinsSingle reports whether the multiplexer's command parser joins
// 'a'\”b' into one word (tmux does; psmux does not).
const quoteJoinsSingle = false

// ShQuote quotes s as one PowerShell word.
func ShQuote(s string) string {
	if s != "" && isPSSafe(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// ShCall is the program word of a PowerShell command line: the call
// operator and the quoted path.
func ShCall(prog string) string { return "& " + ShQuote(filepath.ToSlash(prog)) }

// ShJoin quotes and joins argv as one PowerShell command: & 'prog' 'arg' ...
func ShJoin(argv ...string) string {
	if len(argv) == 0 {
		return ""
	}
	q := make([]string, 0, len(argv)+1)
	q = append(q, ShCall(argv[0]))
	for _, a := range argv[1:] {
		q = append(q, psArg(a))
	}
	return strings.Join(q, " ")
}

// psArg quotes an argument for a native program. Windows PowerShell 5.1
// drops an empty argument and splits one with a double quote in it, so those
// are written in the form that reaches the program intact.
func psArg(a string) string {
	switch {
	case a == "":
		return `'""'`
	case strings.Contains(a, `"`):
		return ShQuote(strings.ReplaceAll(a, `"`, `\"`))
	}
	return ShQuote(a)
}

// ShEnv is put before a PowerShell command line that sets an environment
// variable for it.
func ShEnv(name, value string) string { return "$env:" + name + "=" + ShQuote(value) + "; " }

// isPSSafe reports whether s needs no quotes in PowerShell: letters, digits
// and a few marks that mean nothing to it (no @, $, comma, ;, braces, quotes,
// backtick or a leading - that could read as an operator).
func isPSSafe(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune(`_-./:\%=+`, r):
		default:
			return false
		}
	}
	return true
}

// PowerShell returns the PowerShell psmux runs run-shell commands with
// (pwsh, else Windows PowerShell), so panes and popups use the same one.
func PowerShell() string { return powerShell() }

var powerShell = sync.OnceValue(func() string {
	for _, n := range []string{"pwsh.exe", "powershell.exe"} {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
})

// platformConf makes panes, popups and the plain Terminal use PowerShell,
// the shell imux's command lines are written for (psmux would otherwise
// take $SHELL, which may be Git Bash).
func platformConf() []string {
	return []string{"set -g  default-shell " + Quote(PowerShell())}
}

// platformConfLate replaces settings psmux does not have: its pane borders
// show #{pane_title} only, so imux writes its label as the pane's title
// (and programs may not change it).
func platformConfLate() []string {
	return []string{
		"set -g  pane-border-format ' #{pane_title} '",
		"set -g  allow-set-title off",
	}
}

// psmux340Built is the commit date of psmux 3.4.0. Its status-format has no
// click ranges (#[range=...]); builds after it (still called 3.4.0 until the
// next release) do.
const psmux340Built = "2026-10-13"

// nativeTabs reports whether the tab bar is psmux's own window list: psmux
// 3.4.0 and older have no click ranges, so imux's tab bar and agent bar could
// not be clicked; its window list can.
func nativeTabs(v Version) bool {
	if v.Major != 3 || v.Minor != 4 {
		return v.Major < 3 || v.Major == 3 && v.Minor < 4
	}
	return v.Built == "" || v.Built <= psmux340Built
}

// LegacyPsmux reports whether v is psmux 3.4.0 or older, which stops running
// run-shell commands (all of imux's key bindings and clicks) after a Ctrl+C
// or a click was sent into a pane.
func LegacyPsmux(v Version) bool { return nativeTabs(v) }

// LoginShell is the argv that starts sh in a pane: PowerShell without its
// banner; other shells as they are.
func LoginShell(sh string) []string {
	base := strings.ToLower(filepath.Base(sh))
	if base == "pwsh.exe" || base == "powershell.exe" || base == "pwsh" || base == "powershell" {
		return []string{sh, "-NoLogo"}
	}
	return []string{sh}
}

// FallbackShell is the shell when neither psmux nor $SHELL names one.
func FallbackShell() string { return PowerShell() }
