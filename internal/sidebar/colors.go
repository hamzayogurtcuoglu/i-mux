package sidebar

import (
	"slices"
	"strconv"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/theme"
)

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
	approvedPill       string // an agent whose prompts i-mux answered (D112)
	hoverBg            string // the item under the pointer
	selectBg           string // the selected row (D31)
	boxBg, boxFocusBg  string // the commit box, and with the cursor in it
	barTrack, barThumb string // scroll bars (§9.3 "Scroll bars")
	buttonStyle        string // a filled button (Commit)
	agentFg            [5]string
	// paneBase starts every row (the theme's background and text color),
	// and paneFg / paneBg stand for the terminal's default ones in it; ""
	// on the terminal's colors (theme.PaneBase).
	paneBase, paneFg, paneBg string
	avatars                  []string // projects' avatars (background and text)
	avatarFg                 []string // the same colors as text (AGENTS, T95.1)
)

func init() { theme.Watch(applyPalette) }

func onColor(c theme.Color) string { return theme.OnColor(c) }

func tint(p theme.Palette, c theme.Color, fallback string) string {
	return theme.Tint(p, c, 0.22, fallback)
}

func applyPalette(p theme.Palette) {
	paneBase, paneFg, paneBg = theme.PaneBase(p)
	if p.Classic {
		classic()
		return
	}
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
	approvedPill = tint(p, p.Green, "\x1b[48;5;22m") + p.Green.Fg()
	hoverBg = theme.HoverBg(p).Bg()
	selectBg = p.SelectionBg.Bg()
	boxBg, boxFocusBg = p.Surface0.Bg(), p.Surface1.Bg()
	barTrack = p.Surface0.Fg() + "│" + reset
	barThumb = p.Overlay0.Fg() + "┃" + reset
	buttonStyle = p.Accent.Bg() + onColor(p.Accent)
	agentFg = [5]string{p.Peach.Fg(), p.Blue.Fg(), p.Text.Fg(), p.Mauve.Fg(), p.Yellow.Fg()} // Claude, Antigravity, Codex, Copilot, OpenCode
	// A theme's accent may be one of its colors: each color once, so two
	// projects' colors differ when their avatars do (T95.1).
	var av, fg []string
	for _, c := range []theme.Color{p.Teal, p.Peach, p.Blue, p.Green, p.Mauve, p.Red, p.Yellow, p.Accent} {
		if slices.Contains(fg, c.Fg()) {
			continue
		}
		av = append(av, c.Bg()+onColor(c))
		fg = append(fg, c.Fg())
	}
	avatars, avatarFg = av, fg
}

// classic are the sidebar's colors before themes, exactly (the i-mux theme).
func classic() {
	green, yellow, red, cyan = "\x1b[32m", "\x1b[33m", "\x1b[31m", "\x1b[36m"
	muted, faint = "\x1b[38;5;247m", "\x1b[38;5;244m"
	title = bold + "\x1b[38;5;252m"
	white = "\x1b[38;5;255m"
	tipStyle = white
	selBg, spinFg = "\x1b[48;5;236m", "\x1b[38;5;75m"
	needPill, limitPill = "\x1b[48;5;58m\x1b[38;5;214m", "\x1b[48;5;52m\x1b[38;5;210m"
	approvedPill = "\x1b[48;5;22m\x1b[38;5;114m"
	hoverBg, selectBg = "\x1b[48;5;238m", "\x1b[48;5;24m"
	boxBg, boxFocusBg = "\x1b[48;5;236m", "\x1b[48;5;237m"
	barTrack, barThumb = "\x1b[38;5;236m│"+reset, "\x1b[38;5;245m┃"+reset
	buttonStyle = "\x1b[48;5;24m\x1b[38;5;255m"
	agentFg = [5]string{"\x1b[38;5;173m", "\x1b[38;5;75m", "\x1b[38;5;252m", "\x1b[38;5;141m", "\x1b[38;5;215m"}
	var av, fg []string
	for _, n := range []int{30, 166, 68, 71, 98, 168, 136, 37} {
		av = append(av, "\x1b[48;5;"+strconv.Itoa(n)+"m"+white)
		fg = append(fg, "\x1b[38;5;"+strconv.Itoa(n)+"m")
	}
	avatars, avatarFg = av, fg
}

// paint puts a row on the theme's background (paneBase): the row starts
// with it, and a reset or a default color in it comes back to it.
func paint(row string) string {
	if paneBase == "" {
		return row
	}
	row = strings.NewReplacer(reset, reset+paneBase, "\x1b[39m", paneFg, "\x1b[49m", paneBg).Replace(row)
	return paneBase + row + paneBase
}
