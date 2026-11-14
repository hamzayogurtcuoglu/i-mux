package tmux

import (
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/theme"
)

// TestThemeOptions (T73.3): the palette's colors as @imux_c_* options and
// the styles; popups' from tmux 3.3, menus' from 3.4; the terminal theme in
// tmux's color names.
func TestThemeOptions(t *testing.T) {
	join := func(cs []Cmd) string {
		var out []string
		for _, c := range cs {
			out = append(out, strings.Join(c[2:], " "))
		}
		return strings.Join(out, "\n")
	}
	v := func(s string) Version { x, _ := ParseVersion("tmux " + s); return x }
	got := join(ThemeOptions(theme.Builtin("nord"), v("3.4")))
	for _, want := range []string{
		"@imux_c_accent #88c0d0", "@imux_c_peach #d08770", "@imux_c_sidebar_bg default",
		"status-style bg=#272c36,fg=#d8dee9", "pane-active-border-style bg=#272c36,fg=#88c0d0", "pane-border-style bg=#272c36,fg=#434c5e",
		// Every pane in the theme (D70), with its 16 colors.
		"window-style bg=#272c36,fg=#eceff4", "window-active-style bg=#272c36,fg=#eceff4",
		OptPaneColors + "[1] #bf616a", OptPaneColors + "[4] #81a1c1", OptPaneColors + "[7] #d8dee9", OptPaneColors + "[15] #eceff4",
		"message-style bg=#3b4252,fg=#eceff4", "mode-style bg=#434c5e,fg=#eceff4",
		"popup-style bg=#272c36,fg=#eceff4", "menu-style bg=#272c36,fg=#eceff4", "menu-selected-style bg=#434c5e,fg=#eceff4",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("lacks %q:\n%s", want, got)
		}
	}
	if old := join(ThemeOptions(theme.Builtin("nord"), v("3.2a"))); strings.Contains(old, "popup-") || strings.Contains(old, "menu-") || strings.Contains(old, OptPaneColors) {
		t.Errorf("3.2 gets popup or menu styles:\n%s", old)
	}
	if mid := join(ThemeOptions(theme.Builtin("nord"), v("3.3a"))); !strings.Contains(mid, "popup-style") || strings.Contains(mid, "menu-style") {
		t.Errorf("3.3:\n%s", mid)
	}
	if old := join(ThemeOptions(theme.Builtin("nord"), v("3.2a"))); strings.Contains(old, OptPaneColors) || !strings.Contains(old, "window-style") {
		t.Errorf("3.2: pane colors (3.3+), or no pane colors:\n%s", old)
	}
	// The terminal theme leaves the panes the terminal's colors.
	if term := join(ThemeOptions(theme.Builtin("terminal"), v("3.4"))); !strings.Contains(term, "@imux_c_accent blue") || !strings.Contains(term, "status-style bg=default,fg=brightblack") ||
		strings.Contains(term, "window-style bg") || !strings.Contains(term, "window-style") || !strings.Contains(term, OptPaneColors) {
		t.Errorf("terminal:\n%s", term)
	}
	// The i-mux theme: the tab bar's gray as before, tmux's own borders,
	// messages, popups and menus (unset, so a theme before it goes).
	classic := ThemeOptions(theme.Builtin("i-mux"), v("3.4"))
	got = join(classic)
	if !strings.Contains(got, "status-style bg=default,fg=colour245") || !strings.Contains(got, "@imux_c_subtext0 colour245") || strings.Contains(got, "pane-border-style fg") {
		t.Errorf("classic:\n%s", got)
	}
	unset := 0
	for _, c := range classic {
		if c[1] == "-gu" {
			unset++
		}
	}
	if unset != 13 {
		t.Errorf("classic unsets %d styles, want 13", unset)
	}
	// The tab bar reads the options, so a change needs no new config.
	if f := TabFormat(false); !strings.Contains(f, "#[fg=#{@imux_c_subtext0}]") || strings.Contains(f, "fg=c") {
		t.Errorf("tab bar %q", f)
	}
}

// TestThemeHooks (T73.4): tmux 3.6+ tells imux when the terminal turns
// light or dark; older tmux has no such hooks (they would fail to load).
func TestThemeHooks(t *testing.T) {
	in := confInput("3.6")
	if got := GenerateConf(in); !strings.Contains(got, "set-hook -g client-light-theme \"run-shell -b '/opt/imux/bin/imux __theme appearance light") ||
		!strings.Contains(got, "client-dark-theme") {
		t.Error("3.6: no theme hooks")
	}
	if got := GenerateConf(confInput("3.5a")); strings.Contains(got, "client-light-theme") {
		t.Error("3.5: theme hooks")
	}
	// Following the terminal, the config has the theme for its appearance.
	in.Cfg.Theme = theme.Config{Name: "catppuccin", AutoSwitch: true}
	in.Appearance = theme.AppearanceLight
	if got := GenerateConf(in); !strings.Contains(got, "@imux_c_accent '#1e66f5'") {
		t.Error("light terminal: not Catppuccin Latte")
	}
}
