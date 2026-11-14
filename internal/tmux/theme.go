package tmux

import "github.com/hamzayogurtcuoglu/i-mux/internal/theme"

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
	// The tab bar on the theme's panel, as the sidebar (theme.PaneBase).
	style("status-style", "bg="+p.PanelBg.Tmux()+",fg="+p.Subtext0.Tmux())
	style("pane-border-style", "fg="+p.Surface1.Tmux())
	style("pane-active-border-style", "fg="+p.Accent.Tmux())
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
	out := []string{"pane-border-style", "pane-active-border-style", "message-style", "message-command-style", "mode-style"}
	if v.AtLeast(3, 3) {
		out = append(out, "popup-style", "popup-border-style")
	}
	if v.AtLeast(3, 4) {
		out = append(out, "menu-style", "menu-selected-style", "menu-border-style")
	}
	return out
}
