package tmux

import (
	"strconv"

	"github.com/hamzayogurtcuoglu/i-mux/internal/theme"
)

// OptPaneColors is tmux's palette option for the panes' programs (3.3+).
const OptPaneColors = "pane-colours" //nolint:misspell // tmux's spelling

// OptAppearance is the terminal's light or dark ("light", "dark", "" not
// known), global (D68).
const OptAppearance = "@imux_appearance"

// ColorOpt is the global option holding a palette color for formats (the
// tab bar): "@imux_c_accent" … (D68).
func ColorOpt(token string) string { return "@imux_c_" + token }

// colorRef is a palette color in a format: #{@imux_c_<token>}.
func colorRef(token string) string { return "#{" + ColorOpt(token) + "}" }

// ThemeOptions are the set-option commands that give tmux the palette: the
// colors the tab bar reads, and the styles of the status line, borders,
// messages, the copy mode, popups (tmux 3.3+) and menus (3.4+).
func ThemeOptions(p theme.Palette, v Version) []Cmd {
	var out []Cmd
	for _, t := range theme.Tokens {
		c, _ := p.Get(t)
		v := c.Tmux()
		if p.Classic {
			// The tab bar's grays before themes (only it reads these).
			switch t {
			case "subtext0":
				v = "colour245"
			case "text":
				v = "colour255"
			}
		}
		out = append(out, Cmd{"set-option", "-g", ColorOpt(t), v})
	}
	style := func(opt, s string) { out = append(out, Cmd{"set-option", "-g", opt, s}) }
	if p.Classic {
		// i-mux's look before themes: the tab bar's gray, tmux's own
		// borders, messages, popups and menus.
		style("status-style", "bg=default,fg=colour245")
		for _, o := range classicUnset(v) {
			out = append(out, Cmd{"set-option", "-gu", o})
		}
		return out
	}
	// The whole window in the theme (D70): every pane (the agents' too) on
	// its panel and text color with its 16 colors, as a terminal theme
	// would, the borders and the tab bar on the same panel. A theme on the
	// terminal's colors (terminal) leaves the panes theirs.
	if base, _, _ := theme.PaneBase(p); base != "" {
		pane := "bg=" + p.PanelBg.Tmux() + ",fg=" + p.Text.Tmux()
		style("window-style", pane)
		style("window-active-style", pane)
		if v.AtLeast(3, 3) {
			for i, c := range ANSIColors(p) {
				out = append(out, Cmd{"set-option", "-g", OptPaneColors + "[" + strconv.Itoa(i) + "]", c.Tmux()})
			}
		}
		style("status-style", "bg="+p.PanelBg.Tmux()+",fg="+p.Subtext0.Tmux())
		style("pane-border-style", "bg="+p.PanelBg.Tmux()+",fg="+p.Surface1.Tmux())
		style("pane-active-border-style", "bg="+p.PanelBg.Tmux()+",fg="+p.Accent.Tmux())
	} else {
		for _, o := range paneUnset(v) {
			out = append(out, Cmd{"set-option", "-gu", o})
		}
		style("status-style", "bg=default,fg="+p.Subtext0.Tmux())
		style("pane-border-style", "fg="+p.Surface1.Tmux())
		style("pane-active-border-style", "fg="+p.Accent.Tmux())
	}
	style("message-style", "bg="+p.Surface0.Tmux()+",fg="+p.Text.Tmux())
	style("message-command-style", "bg="+p.Surface0.Tmux()+",fg="+p.Text.Tmux())
	style("mode-style", "bg="+p.SelectionBg.Tmux()+",fg="+p.Text.Tmux())
	if v.AtLeast(3, 3) {
		style("popup-style", "bg="+p.PanelBg.Tmux()+",fg="+p.Text.Tmux())
		style("popup-border-style", "bg="+p.PanelBg.Tmux()+",fg="+p.Surface1.Tmux())
	}
	if v.AtLeast(3, 4) {
		style("menu-style", "bg="+p.PanelBg.Tmux()+",fg="+p.Text.Tmux())
		style("menu-selected-style", "bg="+p.SelectionBg.Tmux()+",fg="+p.Text.Tmux())
		style("menu-border-style", "bg="+p.PanelBg.Tmux()+",fg="+p.Surface1.Tmux())
	}
	return out
}

// classicUnset are the styles the i-mux theme leaves to tmux.
func classicUnset(v Version) []string {
	out := append(paneUnset(v), "pane-border-style", "pane-active-border-style", "message-style", "message-command-style", "mode-style")
	if v.AtLeast(3, 3) {
		out = append(out, "popup-style", "popup-border-style")
	}
	if v.AtLeast(3, 4) {
		out = append(out, "menu-style", "menu-selected-style", "menu-border-style")
	}
	return out
}

// paneUnset are the panes' colors a theme on the terminal's leaves to it.
func paneUnset(v Version) []string {
	out := []string{"window-style", "window-active-style"}
	if v.AtLeast(3, 3) {
		out = append(out, OptPaneColors)
	}
	return out
}

// ANSIColors are the 16 colors the panes' programs get with a theme (D70):
// red, green, yellow, blue, magenta and cyan are the theme's, in both
// intensities; "white" is its secondary text and bright white its text, so
// a program's white reads on a light theme too; black and bright black are
// its border and hint grays.
func ANSIColors(p theme.Palette) []theme.Color {
	six := []theme.Color{p.Red, p.Green, p.Yellow, p.Blue, p.Mauve, p.Teal}
	out := []theme.Color{p.Surface1}
	out = append(out, six...)
	out = append(out, p.Subtext0, p.Overlay0)
	out = append(out, six...)
	return append(out, p.Text)
}
