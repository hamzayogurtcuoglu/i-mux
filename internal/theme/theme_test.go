package theme

import (
	"strings"
	"testing"
)

// TestBuiltins (T73.1): 18 themes, each with all 19 colors, a title, and a
// dark or light look that matches its kind; the default is Catppuccin Mocha.
func TestBuiltins(t *testing.T) {
	if len(Names) != 18 || len(builtins) != 18 || len(Titles) != 18 {
		t.Fatalf("%d names, %d palettes, %d titles", len(Names), len(builtins), len(Titles))
	}
	for _, n := range Names {
		p := Builtin(n)
		if p.Name != n || Titles[n] == "" {
			t.Errorf("%s: %q %q", n, p.Name, Titles[n])
		}
		if !p.SidebarBg.IsReset() {
			t.Errorf("%s: sidebar_bg is not the terminal's", n)
		}
		if n == "terminal" {
			continue
		}
		for _, tok := range Tokens {
			if c, _ := p.Get(tok); tok != "sidebar_bg" && !c.IsRGB() {
				t.Errorf("%s.%s is no 24-bit color", n, tok)
			}
		}
		// A light theme's panel is light and its text dark; a dark one's
		// the other way round.
		if IsLight(n) != p.PanelBg.Light() || IsLight(n) == p.Text.Light() {
			t.Errorf("%s: light %v, panel %s, text %s", n, IsLight(n), p.PanelBg.Tmux(), p.Text.Tmux())
		}
	}
	if p := Builtin("nope"); p.Name != "catppuccin" || p.Accent.Tmux() != "#89b4fa" || p.PanelBg.Tmux() != "#181825" {
		t.Errorf("default %+v", p)
	}
	if p := Builtin("terminal"); p.Accent.Tmux() != "blue" || p.Red.Tmux() != "brightred" || !p.Text.IsReset() {
		t.Errorf("terminal %+v", p)
	}
}

func TestNames(t *testing.T) {
	for in, want := range map[string]string{
		"Catppuccin": "catppuccin", "catppuccin-mocha": "catppuccin", "latte": "catppuccin-latte", "Tokyo Night": "tokyo-night",
		"tokyonight": "tokyo-night", "gruvbox_dark": "gruvbox", "onedark": "one-dark", "dawn": "rose-pine-dawn", "Rose Pine": "rose-pine",
	} {
		if got, ok := Canonical(in); !ok || got != want {
			t.Errorf("Canonical(%q) = %q %v, want %q", in, got, ok, want)
		}
	}
	if _, ok := Canonical("nope"); ok {
		t.Error("an unknown name is a theme")
	}
	for name, want := range map[string][2]string{ // light, dark
		"catppuccin": {"catppuccin-latte", "catppuccin"}, "catppuccin-latte": {"catppuccin-latte", "catppuccin"},
		"gruvbox-light": {"gruvbox-light", "gruvbox"}, "rose-pine": {"rose-pine-dawn", "rose-pine"},
		"nord": {"nord", "nord"}, "terminal": {"terminal", "terminal"},
	} {
		if l, d := LightOf(name), DarkOf(name); l != want[0] || d != want[1] {
			t.Errorf("%s: light %s, dark %s", name, l, d)
		}
	}
}

func TestParseColor(t *testing.T) {
	for in, want := range map[string]string{
		"#a6e3a1": "#a6e3a1", "#ABC": "#aabbcc", "rgb(137, 180, 250)": "#89b4fa", "blue": "blue", "Light_Red": "brightred",
		"darkgray": "brightblack", "gray": "white", "reset": "default", "transparent": "default", "none": "default",
	} {
		c, err := ParseColor(in)
		if err != nil || c.Tmux() != want {
			t.Errorf("ParseColor(%q) = %s %v, want %s", in, c.Tmux(), err, want)
		}
	}
	for _, bad := range []string{"", "#12", "#gggggg", "rgb(1,2)", "rgb(1,2,300)", "chartreuse"} {
		if _, err := ParseColor(bad); err == nil {
			t.Errorf("ParseColor(%q) took it", bad)
		}
	}
	if c := Hex("#89b4fa"); c.Fg() != "\x1b[38;2;137;180;250m" || c.Bg() != "\x1b[48;2;137;180;250m" {
		t.Errorf("rgb escapes %q %q", c.Fg(), c.Bg())
	}
	if ANSI(4).Fg() != "\x1b[34m" || ANSI(9).Fg() != "\x1b[91m" || ANSI(8).Bg() != "\x1b[100m" || Reset.Fg() != "\x1b[39m" || Reset.Bg() != "\x1b[49m" {
		t.Error("ANSI escapes")
	}
	if b := Blend(Hex("#000000"), Hex("#ffffff"), 0.5); b.Tmux() != "#808080" {
		t.Errorf("blend %s", b.Tmux())
	}
}

// TestAppearance (T73.4): a terminal's OSC 11 answer, in its 4, 2 or 1
// hex digits a channel, is light from half the luma up.
func TestAppearance(t *testing.T) {
	for in, want := range map[string]string{
		"rgb:1e1e/1e1e/2e2e": AppearanceDark, "rgb:ffff/ffff/ffff": AppearanceLight, "rgb:ef/f1/f5": AppearanceLight,
		"rgb:0/0/0": AppearanceDark, "rgb:8080/8080/8080": AppearanceLight, "rgb:7f7f/7f7f/7f7f": AppearanceDark,
	} {
		c, ok := ParseOSCColor(in)
		if !ok || AppearanceOf(c) != want {
			t.Errorf("%s: %v %s, want %s", in, ok, AppearanceOf(c), want)
		}
	}
	for _, bad := range []string{"", "rgb:1/2", "rgba:1/2/3/4", "rgb:xx/00/00", "#ffffff"} {
		if _, ok := ParseOSCColor(bad); ok {
			t.Errorf("%q read", bad)
		}
	}
}

// TestResolve (T73.1): the theme, then [theme.custom], then the mode's
// colors; following the terminal, its light or dark sibling (or the named
// one); an unknown appearance is dark.
func TestResolve(t *testing.T) {
	c := Config{Name: "tokyo-night"}
	c.Custom.Accent, c.Custom.Light.Accent, c.Custom.Dark.Text = "#a6e3a1", "red", "#ffffff"
	// A theme of its own: its kind picks the mode's colors (a dark theme
	// stays dark on a light terminal).
	if p := Resolve(c, AppearanceLight); p.Name != "tokyo-night" || p.Accent.Tmux() != "#a6e3a1" || p.Text.Tmux() != "#ffffff" {
		t.Errorf("fixed theme, light terminal: %s %s %s", p.Name, p.Accent.Tmux(), p.Text.Tmux())
	}
	if p := Resolve(Config{Name: "latte", Custom: c.Custom}, AppearanceDark); p.Accent.Tmux() != "red" || p.Text.Tmux() != "#4c4f69" {
		t.Errorf("fixed light theme: %s %s", p.Accent.Tmux(), p.Text.Tmux())
	}
	c.AutoSwitch = true
	if p := Resolve(c, AppearanceLight); p.Name != "tokyo-night-day" || p.Accent.Tmux() != "red" {
		t.Errorf("auto, light: %s %s", p.Name, p.Accent.Tmux())
	}
	if p := Resolve(c, AppearanceDark); p.Name != "tokyo-night" {
		t.Errorf("auto, dark: %s", p.Name)
	}
	c.LightName, c.DarkName = "gruvbox-light", "Nord"
	if a, b := c.Effective(AppearanceLight), c.Effective(""); a != "gruvbox-light" || b != "nord" {
		t.Errorf("named: %s %s", a, b)
	}
	c.LightName = "nope"
	if got := c.Effective(AppearanceLight); got != "tokyo-night-day" {
		t.Errorf("unknown light_name: %s", got)
	}
}

func TestCheck(t *testing.T) {
	c := Config{Name: "nope", LightName: "Latte"}
	c.Custom.Light.Red = "#12"
	w, key, err := c.Check()
	if len(w) != 1 || !strings.Contains(w[0], `theme.name = "nope"; using "catppuccin"`) || key != "theme.custom.light.red" || err == nil {
		t.Errorf("%q %q %v", w, key, err)
	}
	if w, key, err := (Config{Name: "dracula"}).Check(); len(w) != 0 || key != "" || err != nil {
		t.Errorf("good config: %q %q %v", w, key, err)
	}
}

// TestWatch: a watcher gets the palette at once and on each change; the
// same palette again is no change.
func TestWatch(t *testing.T) {
	defer Set(Builtin(Default))
	var got []string
	Watch(func(p Palette) { got = append(got, p.Name) })
	if Set(Builtin("nord")) != true || Set(Builtin("nord")) != false {
		t.Error("Set's report")
	}
	if strings.Join(got, ",") != "catppuccin,nord" {
		t.Errorf("watched %v", got)
	}
	if s := S(); s.Accent != Hex("#88c0d0").Fg() || !strings.HasPrefix(s.Title, "\x1b[1m") {
		t.Errorf("styles %q", s.Accent)
	}
}
