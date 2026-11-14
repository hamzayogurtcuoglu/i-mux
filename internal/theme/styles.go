package theme

// Styles are the escapes the UI draws with, made from a palette (D68): the
// screens take theirs from here, so they are named by what they are for.
type Styles struct {
	Text      string // text
	Title     string // bold text
	Muted     string // secondary text (subtext0)
	Faint     string // hints, line numbers (overlay0)
	Sep       string // separators (surface1)
	Accent    string // links, buttons, the pointer
	Match     string // a typed letter found in a label: bold accent
	Green     string
	Yellow    string
	Red       string
	Blue      string
	Teal      string
	Peach     string
	Mauve     string
	Selected  string // the selected row's background and text
	SelBg     string // the selected row's background only
	ActiveBg  string // the current item's background
	Pill      string // a label on a field (surface1, text)
	Hit       string // a search match: a yellow tint and text
	RedBg     string // removed lines (the diff)
	GreenBg   string // added lines
	EmptyBg   string // the diff's empty side
	OnAccent  string // text on Accent
	AccentBg  string // a filled button
	PanelText string // text on the panel (menus, popups)
}

// Tint is a quiet background of color c: c mixed into the panel's
// background, or fallback (an escape) when either is no 24-bit color.
func Tint(p Palette, c Color, t float64, fallback string) string {
	base := p.PanelBg
	if !base.IsRGB() {
		base = p.SurfaceDim
	}
	if !c.IsRGB() || !base.IsRGB() {
		return fallback
	}
	return Blend(base, c, t).Bg()
}

// OnColor is a readable text color on background c.
func OnColor(c Color) string {
	if !c.IsRGB() {
		return "\x1b[30m"
	}
	if c.Light() {
		return Hex("#11111b").Fg()
	}
	return Hex("#ffffff").Fg()
}

// StylesOf are p's escapes.
func StylesOf(p Palette) Styles {
	s := stylesOf(p)
	if p.Classic {
		// i-mux's own look: the editor's selection blue with white, bold
		// light blue for typed letters, a brown highlight for matches.
		s.Selected = "\x1b[48;5;24m\x1b[38;5;255m"
		s.Match = "\x1b[1m\x1b[38;5;117m"
		s.Hit = "\x1b[48;5;94m\x1b[38;5;230m"
		s.AccentBg = "\x1b[48;5;24m\x1b[38;5;255m"
	}
	return s
}

func stylesOf(p Palette) Styles {
	return Styles{
		Text:      p.Text.Fg(),
		Title:     "\x1b[1m" + p.Text.Fg(),
		Muted:     p.Subtext0.Fg(),
		Faint:     p.Overlay0.Fg(),
		Sep:       p.Surface1.Fg(),
		Accent:    p.Accent.Fg(),
		Match:     "\x1b[1m" + p.Accent.Fg(),
		Green:     p.Green.Fg(),
		Yellow:    p.Yellow.Fg(),
		Red:       p.Red.Fg(),
		Blue:      p.Blue.Fg(),
		Teal:      p.Teal.Fg(),
		Peach:     p.Peach.Fg(),
		Mauve:     p.Mauve.Fg(),
		Selected:  p.SelectionBg.Bg() + p.Text.Fg(),
		SelBg:     p.SelectionBg.Bg(),
		ActiveBg:  p.ActiveRowBg.Bg(),
		Pill:      p.Surface1.Bg() + p.Text.Fg(),
		Hit:       tintFor(p, p.Yellow, 0.35, "\x1b[48;5;94m", need{p.Text, 4.5}) + p.Text.Fg(),
		RedBg:     tintFor(p, p.Red, 0.22, "\x1b[48;5;52m", need{p.Text, 4.5}, need{p.Red, 2.5}),
		GreenBg:   tintFor(p, p.Green, 0.22, "\x1b[48;5;22m", need{p.Text, 4.5}, need{p.Green, 2.5}),
		EmptyBg:   p.SurfaceDim.Bg(),
		OnAccent:  OnColor(p.Accent),
		AccentBg:  p.Accent.Bg() + OnColor(p.Accent),
		PanelText: p.Text.Fg(),
	}
}

// S are the current palette's styles.
func S() Styles { return StylesOf(Current()) }

// PaneBase is what a pane i-mux draws (the sidebar, the "No agents" pane)
// starts each row with: the theme's background and text color, so every
// row reads whatever the terminal's own background is; "" for a theme on
// the terminal's colors (i-mux, terminal). The pane paints it itself, not
// with a tmux style: tmux answers a pane's OSC 11 question with the pane's
// style when it has one, not with the terminal's background.
func PaneBase(p Palette) (base, fg, bg string) {
	b := p.SidebarBg
	if b.IsReset() {
		b = p.PanelBg
	}
	if b.IsReset() || p.Classic {
		return "", "", ""
	}
	fg = "\x1b[39m"
	if !p.Text.IsReset() {
		fg = p.Text.Fg()
	}
	return b.Bg() + fg, fg, b.Bg()
}

// HoverBg is the background of the item under the pointer: halfway from
// the current row's to the selection's, so it is never taken for the
// selection; surface1 on 256 or ANSI colors.
func HoverBg(p Palette) Color {
	if p.ActiveRowBg.IsRGB() && p.SelectionBg.IsRGB() {
		return Blend(p.ActiveRowBg, p.SelectionBg, 0.5)
	}
	return p.Surface1
}
