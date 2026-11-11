//go:build windows

package cli

import (
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/winx"
)

func init() {
	register(&command{
		Name:   "__mouse3",
		Args:   "--pane P --x X --y Y --app 0|1",
		Hidden: true,
		Run:    runMouse3,
	})
}

// runMouse3 handles a right-click in a pane under psmux (see the
// MouseDown3Pane binding in tmux.platformConfLate): a sidebar, or a program
// that turned on mouse reporting, gets the click as an SGR mouse report at
// the pane-relative cell; any other pane gets the clipboard's text pasted.
func runMouse3(e *Env, args []string) error {
	fs := newFlags(e, "__mouse3")
	paneF := fs.String("pane", "", "the pane under the mouse")
	xF := fs.Int("x", 0, "column in the pane (0-based)")
	yF := fs.Int("y", 0, "row in the pane (0-based)")
	appF := fs.String("app", "0", "1 when the pane takes the mouse itself")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	pane, err := targetPane(*paneF)
	if err != nil {
		return err
	}
	t := tmux.New(paths.TmuxConf())
	ctx := context.Background()
	if *appF == "1" {
		x, y := strconv.Itoa(*xF+1), strconv.Itoa(*yF+1)
		_, err := t.Run(ctx, tmux.Cmd{"send-keys", "-t", pane, "-l", "\x1b[<2;" + x + ";" + y + "M\x1b[<2;" + x + ";" + y + "m"})
		return err
	}
	text, err := clipboardText(ctx)
	if err != nil || text == "" {
		return err
	}
	_, err = t.Run(ctx,
		tmux.Cmd{"set-buffer", "-b", "imux-paste", "--", text},
		tmux.Cmd{"paste-buffer", "-b", "imux-paste", "-d", "-p", "-t", pane})
	return err
}

// clipboardText reads the Windows clipboard's text (UTF-8 through base64, so
// the console code page cannot change it).
func clipboardText(ctx context.Context) (string, error) {
	const script = `$t = Get-Clipboard -Raw; if ($t) { [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($t)) }`
	cmd := winx.Hidden(exec.CommandContext(ctx, winx.WindowsPowerShell(), "-NoProfile", "-NonInteractive", "-Command", script))
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("clipboard: %w", err)
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
	if err != nil {
		return "", fmt.Errorf("clipboard: %w", err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n"), nil
}
