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
		Hit:       Tint(p, p.Yellow, 0.35, "\x1b[48;5;94m") + p.Text.Fg(),
		RedBg:     Tint(p, p.Red, 0.22, "\x1b[48;5;52m"),
		GreenBg:   Tint(p, p.Green, 0.22, "\x1b[48;5;22m"),
		EmptyBg:   p.SurfaceDim.Bg(),
		OnAccent:  OnColor(p.Accent),
		AccentBg:  p.Accent.Bg() + OnColor(p.Accent),
		PanelText: p.Text.Fg(),
	}
}

// S are the current palette's styles.
func S() Styles { return StylesOf(Current()) }
