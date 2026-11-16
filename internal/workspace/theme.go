package workspace

import (
	"context"
	"errors"
	"strings"
	"syscall"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/theme"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// Appearance is the terminal's light or dark as tmux keeps it
// (@imux_appearance, D68): "light", "dark", or "" when not known or no
// server runs.
func Appearance(ctx context.Context, t *tmux.Client) string {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	out, err := t.Run(ctx, tmux.Cmd{"show-options", "-gqv", tmux.OptAppearance})
	if err != nil {
		return ""
	}
	switch a := strings.TrimSpace(out); a {
	case theme.AppearanceLight, theme.AppearanceDark:
		return a
	}
	return ""
}

// Palette is the palette for the config and the terminal's appearance.
func (w *WS) Palette(ctx context.Context) theme.Palette {
	a := ""
	if w.Cfg.Theme.AutoSwitch {
		a = Appearance(ctx, w.T)
	}
	return theme.Resolve(w.Cfg.Theme, a)
}

// SetAppearance notes that the terminal is light or dark (tmux's
// client-light-theme / client-dark-theme hooks, a sidebar's OSC 11 query).
// When it changed and the theme follows it, tmux's styles take the other
// theme and the sidebars redraw with it. It reports whether it changed.
func (w *WS) SetAppearance(ctx context.Context, appearance string) (bool, error) {
	if appearance != theme.AppearanceLight && appearance != theme.AppearanceDark {
		return false, nil
	}
	if Appearance(ctx, w.T) == appearance {
		return false, nil
	}
	if _, err := w.T.Run(ctx, tmux.Cmd{"set-option", "-g", tmux.OptAppearance, appearance}); err != nil {
		return false, err
	}
	if !w.Cfg.Theme.AutoSwitch {
		return true, nil
	}
	p := theme.Resolve(w.Cfg.Theme, appearance)
	theme.Set(p)
	if err := w.ApplyTheme(ctx, p); err != nil {
		return true, err
	}
	w.RefreshSidebars(ctx)
	return true, nil
}

// ApplyTheme gives a running tmux the palette's colors and styles (the
// same lines tmux.conf has), and the attached terminals its background
// (D77) first, before tmux redraws them.
func (w *WS) ApplyTheme(ctx context.Context, p theme.Palette) error {
	v, err := w.T.BinVersion(ctx)
	if err != nil {
		return err
	}
	bg := tmux.TerminalBg(p, w.Cfg.Theme.AutoSwitch)
	w.PaintTerminals(ctx, bg)
	cmds := append(tmux.ThemeOptions(p, v), tmux.Cmd{"set-option", "-g", tmux.TermBgItem, tmux.TerminalBgOverride(bg)})
	_, err = w.T.Run(ctx, cmds...)
	return err
}

// PaintTerminals gives the attached clients' terminals background bg now
// (OSC 11), or their own back for "" (OSC 111), D77; a client that attaches
// later gets it from terminal-overrides. A control-mode client has no
// terminal of its own to paint.
func (w *WS) PaintTerminals(ctx context.Context, bg string) {
	out, err := w.T.Run(ctx, tmux.Cmd{"list-clients", "-F", "#{client_control_mode}\t#{client_tty}"})
	if err != nil {
		return
	}
	seq := tmux.TerminalBgSeq(bg)
	for _, r := range tmux.Rows(out, 2) {
		if r[0] != "1" && strings.HasPrefix(r[1], "/dev/") {
			_ = writeTerminal(r[1], seq)
		}
	}
}

// writeTerminal writes s to the terminal device tty in one blocking write:
// the kernel keeps it whole, so tmux's own output cannot land inside it (a
// non-blocking file may write it in parts). A terminal that does not read
// is given up on after a second. Tests replace it.
var writeTerminal = func(tty, s string) error {
	done := make(chan error, 1)
	go func() {
		fd, err := syscall.Open(tty, syscall.O_WRONLY|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
		if err != nil {
			done <- err
			return
		}
		defer func() { _ = syscall.Close(fd) }()
		b := []byte(s)
		for len(b) > 0 {
			n, err := syscall.Write(fd, b)
			if err != nil && !errors.Is(err, syscall.EINTR) {
				done <- err
				return
			}
			if n > 0 {
				b = b[n:]
			}
		}
		done <- nil
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(time.Second):
		return errors.New("terminal not reading: " + tty)
	}
}
