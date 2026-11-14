package theme

import "testing"

// TestLegible (D68): in every built-in 24-bit theme, i-mux's text reads on
// every background it draws it on: text 4.5:1 (WCAG AA), secondary text
// 3:1, hints 2.2:1, the accent and state colors 2.5:1; the selection,
// fields and borders stand out from the panel; search matches and the
// diff's red and green lines keep their text readable.
func TestLegible(t *testing.T) {
	for _, n := range Names {
		p := Builtin(n)
		if !p.PanelBg.IsRGB() {
			continue // on the terminal's colors (i-mux, terminal)
		}
		hover := Blend(p.ActiveRowBg, p.SelectionBg, 0.5)
		hit, _ := tintColor(p, p.Yellow, 0.35, need{p.Text, 4.5})
		red, _ := tintColor(p, p.Red, 0.22, need{p.Text, 4.5}, need{p.Red, 2.5})
		green, _ := tintColor(p, p.Green, 0.22, need{p.Text, 4.5}, need{p.Green, 2.5})
		for _, c := range []struct {
			what   string
			fg, bg Color
			min    float64
		}{
			{"text on the panel", p.Text, p.PanelBg, 4.5},
			{"text on the selection", p.Text, p.SelectionBg, 4.5},
			{"text on the current row", p.Text, p.ActiveRowBg, 4.5},
			{"text under the pointer", p.Text, hover, 4.5},
			{"text in a field", p.Text, p.Surface0, 4.5},
			{"text on a pill", p.Text, p.Surface1, 4.5},
			{"secondary text", p.Subtext0, p.PanelBg, 3},
			{"secondary text on the current row", p.Subtext0, p.ActiveRowBg, 3},
			{"secondary text on the selection", p.Subtext0, p.SelectionBg, 3},
			{"hints", p.Overlay0, p.PanelBg, 2.2},
			{"the tab's ×", p.Overlay1, p.Surface0, 2.5},
			{"the accent", p.Accent, p.PanelBg, 3},
			{"the accent on the selection", p.Accent, p.SelectionBg, 2.5},
			{"green", p.Green, p.PanelBg, 2.5},
			{"yellow", p.Yellow, p.PanelBg, 2.5},
			{"red", p.Red, p.PanelBg, 2.5},
			{"blue", p.Blue, p.PanelBg, 2.5},
			{"peach", p.Peach, p.PanelBg, 2.5},
			{"mauve", p.Mauve, p.PanelBg, 2.5},
			{"teal", p.Teal, p.PanelBg, 2.5},
			{"a search match", p.Text, hit, 4.5},
			{"a removed line", p.Text, red, 4.5},
			{"an added line", p.Text, green, 4.5},
			{"the removed mark", p.Red, red, 2.5},
			{"the added mark", p.Green, green, 2.5},
			{"the selection against the panel", p.SelectionBg, p.PanelBg, 1.25},
			{"a field against the panel", p.Surface0, p.PanelBg, 1.1},
			{"the pointer's row against the panel", hover, p.PanelBg, 1.08},
		} {
			if r := Contrast(c.fg, c.bg); r < c.min {
				t.Errorf("%s: %s %.2f:1 < %.1f (%s on %s)", n, c.what, r, c.min, c.fg.Tmux(), c.bg.Tmux())
			}
		}
	}
}

// TestLegibleKeepsSchemes: colors that already read are the scheme's own
// (Catppuccin Mocha's text and accent; Gruvbox's yellow).
func TestLegibleKeepsSchemes(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{Builtin("catppuccin").Text.Tmux(), "#cdd6f4"},
		{Builtin("catppuccin").Accent.Tmux(), "#89b4fa"},
		{Builtin("gruvbox").Yellow.Tmux(), "#fabd2f"},
		{Builtin("tokyo-night").PanelBg.Tmux(), "#1a1b26"},
	} {
		if c.got != c.want {
			t.Errorf("%s, want %s", c.got, c.want)
		}
	}
}
