package sidebar

import "github.com/hamzayogurtcuoglu/i-mux/internal/theme"

// The sidebar's colors, from the theme's palette (D68); applyPalette sets
// them again when it changes.
var (
	green, yellow, red string
	cyan               string // links, buttons, the focus mark: the accent
	muted              string // secondary text
	faint              string // hints
	title              string // section titles: bold text
	white              string // text that stands out
	tipStyle           string // a button's label in its title (D63)
	selBg              string // the current agent's row
	spinFg             string // a working agent's spinner
	needPill           string // an agent that needs you
	limitPill          string // a limited agent
	hoverBg            string // the item under the pointer
	selectBg           string // the selected row (D31)
	boxBg, boxFocusBg  string // the commit box, and with the cursor in it
	barTrack, barThumb string // scroll bars (§9.3 "Scroll bars")
	buttonStyle        string // a filled button (Commit)
	agentFg            [5]string
	avatars            []string // projects' avatars (background and text)
)

func init() { theme.Watch(applyPalette) }

func onColor(c theme.Color) string { return theme.OnColor(c) }

func tint(p theme.Palette, c theme.Color, fallback string) string {
	return theme.Tint(p, c, 0.22, fallback)
}

func applyPalette(p theme.Palette) {
	green, yellow, red = p.Green.Fg(), p.Yellow.Fg(), p.Red.Fg()
	cyan = p.Accent.Fg()
	muted, faint = p.Subtext0.Fg(), p.Overlay0.Fg()
	title = bold + p.Text.Fg()
	white = p.Text.Fg()
	tipStyle = white
	selBg = p.ActiveRowBg.Bg()
	spinFg = p.Blue.Fg()
	needPill = tint(p, p.Yellow, "\x1b[48;5;58m") + p.Yellow.Fg()
	limitPill = tint(p, p.Red, "\x1b[48;5;52m") + p.Red.Fg()
	// Hover: halfway from the current row's background to the selection's,
	// so it is never taken for the selection (the same as surface0 in some).
	hoverBg = p.Surface0.Bg()
	if p.ActiveRowBg.IsRGB() && p.SelectionBg.IsRGB() {
		hoverBg = theme.Blend(p.ActiveRowBg, p.SelectionBg, 0.5).Bg()
	}
	selectBg = p.SelectionBg.Bg()
	boxBg, boxFocusBg = p.Surface0.Bg(), p.Surface1.Bg()
	barTrack = p.Surface0.Fg() + "│" + reset
	barThumb = p.Overlay0.Fg() + "┃" + reset
	buttonStyle = p.Accent.Bg() + onColor(p.Accent)
	agentFg = [5]string{p.Peach.Fg(), p.Blue.Fg(), p.Text.Fg(), p.Mauve.Fg(), p.Yellow.Fg()} // Claude, Antigravity, Codex, Copilot, OpenCode
	var av []string
	for _, c := range []theme.Color{p.Teal, p.Peach, p.Blue, p.Green, p.Mauve, p.Red, p.Yellow, p.Accent} {
		av = append(av, c.Bg()+onColor(c))
	}
	avatars = av
}
