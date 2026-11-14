package workspace

import (
	"context"
	"strings"
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
// same lines tmux.conf has).
func (w *WS) ApplyTheme(ctx context.Context, p theme.Palette) error {
	v, err := w.T.BinVersion(ctx)
	if err != nil {
		return err
	}
	_, err = w.T.Run(ctx, tmux.ThemeOptions(p, v)...)
	return err
}
