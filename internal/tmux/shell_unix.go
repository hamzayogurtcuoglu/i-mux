//go:build !windows

package tmux

import "strings"

// Silent is appended to every shell command imux's bindings, hooks and menu
// items run: tmux shows any output of run-shell (or a non-zero exit) in the
// pane's view mode, so imux reports results with display-message instead.
const Silent = " >/dev/null 2>&1 || true"

// quoteJoinsSingle reports whether tmux's command parser joins 'a'\”b'
// into one word.
const quoteJoinsSingle = true

// ShQuote quotes s for sh (run-shell passes its command to /bin/sh -c).
func ShQuote(s string) string {
	if s != "" && isSafe(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// ShCall is the program word of a shell command line: the quoted path.
func ShCall(prog string) string { return ShQuote(prog) }

// ShJoin quotes and joins argv for sh.
func ShJoin(argv ...string) string {
	q := make([]string, len(argv))
	for i, a := range argv {
		q[i] = ShQuote(a)
	}
	return strings.Join(q, " ")
}

// ShEnv is put before a shell command line that sets environment
// variables: sh takes them as `NAME=value cmd`.
func ShEnv(name, value string) string { return name + "=" + ShQuote(value) + " " }

// platformConf is extra tmux.conf lines this system needs (none on Unix).
func platformConf() []string { return nil }

// platformConfLate is extra lines after imux's own (none on Unix).
func platformConfLate() []string { return nil }

// LoginShell is the argv that starts sh as a login shell in a pane.
func LoginShell(sh string) []string { return []string{sh, "-l"} }

// FallbackShell is the shell when neither tmux nor $SHELL names one.
func FallbackShell() string { return "/bin/sh" }
