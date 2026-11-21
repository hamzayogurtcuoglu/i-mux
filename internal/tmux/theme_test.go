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
	if f := TabFormat(); !strings.Contains(f, "#[fg=#{@imux_c_subtext0}]") || strings.Contains(f, "fg=c") {
		t.Errorf("tab bar %q", f)
	}
}

// TestTerminalBg (T82.1): the terminal's own background is the panel of a
// theme on its own 24-bit colors that does not follow the terminal (D77),
// set with tmux's Enfcs and given back with its Dsfcs; with the other
// themes it is only given back. The color's ":" is doubled, as overrides
// are separated by ":".
func TestTerminalBg(t *testing.T) {
	nord := TerminalBg(theme.Builtin("nord"), false)
	if nord != "rgb:27/2c/36" {
		t.Fatalf("nord %q", nord)
	}
	for name, follow := range map[string]bool{"nord": true, "i-mux": false, "terminal": false} {
		if bg := TerminalBg(theme.Builtin(name), follow); bg != "" {
			t.Errorf("%s (following the terminal: %v): %q", name, follow, bg)
		}
	}
	if got, want := TerminalBgOverride(nord), `*:Enfcs=\E[?1004h\E]11;rgb::27/2c/36\007:Dsfcs=\E]111\007\E[?1004l`; got != want {
		t.Errorf("override %q, want %q", got, want)
	}
	if got := TerminalBgOverride(""); got != `*:Dsfcs=\E]111\007\E[?1004l` {
		t.Errorf("override with no background %q", got)
	}
	if TerminalBgSeq(nord) != "\x1b]11;rgb:27/2c/36\x07" || TerminalBgSeq("") != "\x1b]111\x07" {
		t.Errorf("escapes %q %q", TerminalBgSeq(nord), TerminalBgSeq(""))
	}
}

// TestThemeHooks (T73.4): tmux 3.6+ tells imux when the terminal turns
// light or dark; older tmux has no such hooks (they would fail to load).
func TestThemeHooks(t *testing.T) {
	in := confInput("3.8")
	if got := GenerateConf(in); !strings.Contains(got, "set-hook -g client-light-theme \"run-shell -b '/opt/imux/bin/imux __theme appearance light") ||
		!strings.Contains(got, "client-dark-theme") {
		t.Error("no theme hooks")
	}
	// Following the terminal, the config has the theme for its appearance.
	in.Cfg.Theme = theme.Config{Name: "catppuccin", AutoSwitch: true}
	in.Appearance = theme.AppearanceLight
	if got := GenerateConf(in); !strings.Contains(got, "@imux_c_accent '#1e66f5'") {
		t.Error("light terminal: not Catppuccin Latte")
	}
}
