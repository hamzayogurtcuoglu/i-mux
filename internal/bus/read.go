package bus

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

var ansiRe = regexp.MustCompile("\x1b(?:\\[[0-9;?]*[ -/]*[@-~]|\\][^\x07\x1b]*(?:\x07|\x1b\\\\)|[()][0-9A-Za-z]|[=>78DEHMNOc])")

// StripANSI removes terminal escape sequences.
func StripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

// Read returns the last n lines of pane's screen and scrollback, wrapped
// lines joined, escapes stripped and trailing blank lines dropped.
func (b *Bus) Read(ctx context.Context, pane string, n int) (string, error) {
	if n <= 0 {
		n = 60
	}
	out, err := b.W.T.Run(ctx, tmux.Cmd{"capture-pane", "-p", "-J", "-t", pane, "-S", "-" + strconv.Itoa(n)})
	if err != nil {
		return "", err
	}
	return LastLines(StripANSI(out), n), nil
}

// LastLines keeps the last n lines of s after trimming trailing blank
// lines (and trailing spaces on each line).
func LastLines(s string, n int) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r", ""), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}
