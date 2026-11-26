package phone

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/theme"
)

// Look is the desktop's theme as the app takes it (D110): each token's
// #rrggbb ("accent", "panel_bg"…), and Light only when the theme follows
// the terminal's light/dark, for a phone that is light.
type Look struct {
	Colors map[string]string `json:"colors,omitempty"`
	Light  map[string]string `json:"light,omitempty"`
}

// LookOf is theme config c as the phone shows it.
func LookOf(c theme.Config) Look {
	l := Look{Colors: Colors(theme.Resolve(c, theme.AppearanceDark))}
	if c.AutoSwitch {
		l.Light = Colors(theme.Resolve(c, theme.AppearanceLight))
	}
	return l
}

// Colors are p's colors as #rrggbb, by token: a 256 color as xterm draws
// it; the terminal's own (ANSI, reset) are left out, so the app keeps its
// own there. Nil when none is left.
func Colors(p theme.Palette) map[string]string {
	out := map[string]string{}
	for _, t := range theme.Tokens {
		c, _ := p.Get(t)
		if h := hexOf(c.Tmux()); h != "" {
			out[t] = h
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// hexOf turns a tmux color ("#rrggbb", "colourN") into #rrggbb; "" for
// the terminal's own.
func hexOf(tmuxColor string) string {
	if strings.HasPrefix(tmuxColor, "#") && len(tmuxColor) == 7 {
		return tmuxColor
	}
	n, err := strconv.Atoi(strings.TrimPrefix(tmuxColor, "colour")) //nolint:misspell // tmux's spelling
	if err != nil || !strings.HasPrefix(tmuxColor, "colour") || n < 0 || n > 255 {
		return ""
	}
	return xterm(n)
}

// xterm16 are xterm's own first 16 colors.
var xterm16 = []string{
	"#000000", "#cd0000", "#00cd00", "#cdcd00", "#0000ee", "#cd00cd", "#00cdcd", "#e5e5e5",
	"#7f7f7f", "#ff0000", "#00ff00", "#ffff00", "#5c5cff", "#ff00ff", "#00ffff", "#ffffff",
}

// xterm is 256-color n as xterm draws it.
func xterm(n int) string {
	switch {
	case n < 16:
		return xterm16[n]
	case n < 232:
		n -= 16
		l := []int{0, 95, 135, 175, 215, 255}
		return fmt.Sprintf("#%02x%02x%02x", l[n/36], l[n/6%6], l[n%6])
	}
	g := 8 + (n-232)*10
	return fmt.Sprintf("#%02x%02x%02x", g, g, g)
}
