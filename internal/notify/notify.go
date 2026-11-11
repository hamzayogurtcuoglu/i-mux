// Package notify shows a desktop notification on every platform i-mux runs
// on (ARCHITECTURE "Notifications"): a configured command, macOS's
// notification center, a Windows toast, the Linux desktop's (notify-send,
// else D-Bus), or the terminal's own (OSC 9 / OSC 777) when there is no
// desktop.
package notify

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/winx"
)

// Message is one notification.
type Message struct {
	Title, Body string
	Sound       bool
}

// Client is a terminal attached to tmux: where a terminal notification is
// written.
type Client struct {
	TTY     string // /dev/ttys003
	Program string // TERM_PROGRAM of its terminal ("iTerm.app", "ghostty", …)
}

// Notifier delivers messages. The zero value works; tests replace the
// fields.
type Notifier struct {
	Command  []string // [notify] command (split); the title and body are appended
	GOOS     string   // runtime.GOOS
	LookPath func(string) (string, error)
	Run      func(ctx context.Context, name string, args ...string) error
	// OpenTTY opens a client's terminal for writing.
	OpenTTY func(path string) (interface {
		Write([]byte) (int, error)
		Close() error
	}, error)
}

func (n *Notifier) goos() string {
	if n.GOOS != "" {
		return n.GOOS
	}
	return runtime.GOOS
}

func (n *Notifier) lookPath(name string) bool {
	lp := n.LookPath
	if lp == nil {
		lp = exec.LookPath
	}
	_, err := lp(name)
	return err == nil
}

func (n *Notifier) run(ctx context.Context, name string, args ...string) error {
	if n.Run != nil {
		return n.Run(ctx, name, args...)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return winx.Hidden(exec.CommandContext(ctx, name, args...)).Run()
}

// ErrNone means no way to notify worked; the caller falls back to the
// terminals.
var ErrNone = errors.New("no notification service")

// System shows m through the configured command or the desktop. It returns
// the way it used ("command", "osascript", "toast", "notify-send", "gdbus").
func (n *Notifier) System(ctx context.Context, m Message) (string, error) {
	if len(n.Command) > 0 {
		args := append(append([]string{}, n.Command[1:]...), m.Title, m.Body)
		return "command", n.run(ctx, n.Command[0], args...)
	}
	switch n.goos() {
	case "darwin":
		if !n.lookPath("osascript") {
			return "", ErrNone
		}
		script := "display notification " + appleString(m.Body) + " with title " + appleString(m.Title)
		if m.Sound {
			script += ` sound name "Glass"`
		}
		return "osascript", n.run(ctx, "osascript", "-e", script)
	case "windows":
		if !n.lookPath("powershell.exe") {
			return "", ErrNone
		}
		return "toast", n.run(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", toastScript(m))
	default:
		if n.lookPath("notify-send") {
			args := []string{"-a", "i-mux"}
			if m.Sound {
				args = append(args, "-h", "string:sound-name:message-new-instant")
			}
			if err := n.run(ctx, "notify-send", append(args, m.Title, m.Body)...); err == nil {
				return "notify-send", nil
			}
		}
		if n.lookPath("gdbus") {
			err := n.run(ctx, "gdbus", "call", "--session",
				"--dest", "org.freedesktop.Notifications",
				"--object-path", "/org/freedesktop/Notifications",
				"--method", "org.freedesktop.Notifications.Notify",
				"i-mux", "0", "", m.Title, m.Body, "[]", "{}", "8000")
			if err == nil {
				return "gdbus", nil
			}
		}
		return "", ErrNone
	}
}

// appleString quotes s for AppleScript.
func appleString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// clean turns control characters (which would end an escape sequence
// early) into spaces.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
}

// OSC is the terminal notification for a terminal program: OSC 9 for
// iTerm2, OSC 777 for the others that have one (Ghostty, WezTerm, foot,
// VTE terminals, kitty).
func OSC(program string, m Message) string {
	title, body := clean(m.Title), clean(m.Body)
	if strings.Contains(strings.ToLower(program), "iterm") {
		return "\x1b]9;" + title + ": " + body + "\a"
	}
	return "\x1b]777;notify;" + strings.ReplaceAll(title, ";", ",") + ";" + body + "\a"
}

// Terminals writes m as a terminal notification to each client.
func (n *Notifier) Terminals(clients []Client, m Message) int {
	open := n.OpenTTY
	if open == nil {
		open = func(p string) (interface {
			Write([]byte) (int, error)
			Close() error
		}, error,
		) {
			return os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0)
		}
	}
	sent := 0
	for _, c := range clients {
		if c.TTY == "" {
			continue
		}
		f, err := open(c.TTY)
		if err != nil {
			continue
		}
		if _, err := f.Write([]byte(OSC(c.Program, m))); err == nil {
			sent++
		}
		_ = f.Close()
	}
	return sent
}
