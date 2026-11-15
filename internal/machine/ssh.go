package machine

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
)

// SSHBin is the ssh program: $IMUX_SSH (tests), else ssh on the PATH.
func SSHBin() string {
	if b := os.Getenv("IMUX_SSH"); b != "" {
		return b
	}
	return "ssh"
}

// ControlDir holds the master connections' sockets (0700).
func ControlDir() string { return filepath.Join(paths.Home(), "ssh") }

// Mode is how an ssh command runs.
type Mode int

const (
	// Background never asks the user anything (BatchMode): the watcher's
	// questions.
	Background Mode = iota
	// Interactive may ask (an unknown host's key, a password) in the
	// user's terminal: adding a machine, reconnecting it.
	Interactive
	// Attach is Interactive with a terminal for the machine's i-mux.
	Attach
)

// Options are the ssh options every command gets (D73): one master
// connection per machine that each command rides on; dead connections
// noticed; nothing of this computer forwarded to the machine. Host keys
// are checked as the user's configuration says: StrictHostKeyChecking is
// never set.
func Options(mode Mode) []string {
	batch := "yes"
	if mode != Background {
		batch = "no"
	}
	o := []string{
		"-o", "BatchMode=" + batch,
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + filepath.Join(ControlDir(), "%C"),
		"-o", "ControlPersist=600",
		"-o", "ConnectTimeout=10",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=3",
		"-o", "ForwardAgent=no",
		"-o", "ForwardX11=no",
		"-o", "ClearAllForwardings=yes",
		"-o", "PermitLocalCommand=no",
	}
	if mode == Attach {
		return append(o, "-t")
	}
	return append(o, "-T")
}

// findImux is the shell script that runs the machine's i-mux with the
// script's arguments: on the PATH, else where its installers put it.
const findImux = `for i in imux "$HOME/.local/bin/imux" /opt/homebrew/bin/imux /usr/local/bin/imux; do ` +
	`if command -v "$i" >/dev/null 2>&1; then exec "$i" "$@"; fi; done; echo "i-mux is not installed on this machine" >&2; exit 127`

// RemoteCommand is the command line the machine's shell runs for i-mux
// with words: ssh joins its arguments with spaces and the login shell
// parses them, so every word is quoted. The words are i-mux's own or were
// checked (ValidSession, ValidPane) or are data quoted here (a folder).
func RemoteCommand(words ...string) string {
	q := []string{"exec", "sh", "-c", shQuote(findImux), "imux"}
	for _, w := range words {
		q = append(q, shQuote(w))
	}
	return strings.Join(q, " ")
}

// Args is the whole ssh argv (without the program) that runs i-mux with
// words on m: the destination after "--", so it is never read as an
// option.
func Args(m Machine, mode Mode, words ...string) []string {
	a := append(Options(mode), "--", m.Target)
	if len(words) > 0 {
		a = append(a, RemoteCommand(words...))
	}
	return a
}

// ShellArgs is the ssh argv for a plain terminal on m.
func ShellArgs(m Machine) []string {
	return append(Options(Attach), "--", m.Target)
}

// ExitArgs closes m's master connection (Reconnect, Remove).
func ExitArgs(m Machine) []string {
	return []string{"-o", "ControlPath=" + filepath.Join(ControlDir(), "%C"), "-O", "exit", "--", m.Target}
}

// shQuote is s in single quotes for a POSIX shell.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

var (
	sessionRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,63}$`) // never an option
	paneRe    = regexp.MustCompile(`^%[0-9]{1,9}$`)
)

// ValidSession reports a session name a machine may send and i-mux may
// pass back to it.
func ValidSession(s string) bool { return sessionRe.MatchString(s) }

// ValidPane reports a pane id ("%3").
func ValidPane(p string) bool { return paneRe.MatchString(p) }
