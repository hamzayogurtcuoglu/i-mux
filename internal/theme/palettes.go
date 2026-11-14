package theme

import (
	"fmt"
	"strings"
)

// Palette is a theme's 19 named colors (D68).
type Palette struct {
	Name        string
	Accent      Color // buttons, links, the focus mark, typed matches, the active border
	PanelBg     Color // menus' and popups' background
	SidebarBg   Color // the sidebar's background (reset: the terminal's)
	ActiveRowBg Color // the current agent's row
	SelectionBg Color // the selected row
	Surface0    Color // fields, scroll bar tracks, the current tab
	Surface1    Color // borders, separators, a focused field
	SurfaceDim  Color // fillers (the diff's empty side)
	Overlay0    Color // hints
	Overlay1    Color // quieter text
	Text        Color
	Subtext0    Color // secondary text
	Mauve       Color
	Green       Color
	Yellow      Color
	Red         Color
	Blue        Color
	Teal        Color
	Peach       Color
}

// Tokens are the colors' names in the config ([theme.custom]), in order.
var Tokens = []string{
	"accent", "panel_bg", "sidebar_bg", "active_row_bg", "selection_bg", "surface0", "surface1",
	"surface_dim", "overlay0", "overlay1", "text", "subtext0", "mauve", "green", "yellow", "red",
	"blue", "teal", "peach",
}

// colors are p's colors in Tokens' order, to set by name.
func (p *Palette) colors() []*Color {
	return []*Color{
		&p.Accent, &p.PanelBg, &p.SidebarBg, &p.ActiveRowBg, &p.SelectionBg, &p.Surface0, &p.Surface1,
		&p.SurfaceDim, &p.Overlay0, &p.Overlay1, &p.Text, &p.Subtext0, &p.Mauve, &p.Green, &p.Yellow, &p.Red,
		&p.Blue, &p.Teal, &p.Peach,
	}
}

// Get is the color named token ("accent"…).
func (p Palette) Get(token string) (Color, bool) {
	for i, t := range Tokens {
		if t == token {
			return *p.colors()[i], true
		}
	}
	return Color{}, false
}

// Default is the theme without a config: Catppuccin Mocha.
const Default = "catppuccin"

// Names are the built-in themes, dark ones next to their light siblings.
var Names = []string{
	"catppuccin", "catppuccin-latte", "terminal", "tokyo-night", "tokyo-night-day", "dracula", "nord",
	"gruvbox", "gruvbox-light", "one-dark", "one-light", "solarized", "solarized-light", "kanagawa",
	"kanagawa-lotus", "rose-pine", "rose-pine-dawn", "vesper",
}

// Titles are the themes' names as menus show them.
var Titles = map[string]string{
	"catppuccin": "Catppuccin", "catppuccin-latte": "Catppuccin Latte", "terminal": "Terminal",
	"tokyo-night": "Tokyo Night", "tokyo-night-day": "Tokyo Night Day", "dracula": "Dracula", "nord": "Nord",
	"gruvbox": "Gruvbox", "gruvbox-light": "Gruvbox Light", "one-dark": "One Dark", "one-light": "One Light",
	"solarized": "Solarized", "solarized-light": "Solarized Light", "kanagawa": "Kanagawa",
	"kanagawa-lotus": "Kanagawa Lotus", "rose-pine": "Rosé Pine", "rose-pine-dawn": "Rosé Pine Dawn",
	"vesper": "Vesper",
}

var aliases = map[string]string{
	"catppuccin-mocha": "catppuccin", "mocha": "catppuccin", "latte": "catppuccin-latte", "light": "catppuccin-latte",
	"tokyonight": "tokyo-night", "tokyo-day": "tokyo-night-day", "tokyonight-day": "tokyo-night-day",
	"gruvbox-dark": "gruvbox", "solarized-dark": "solarized", "onedark": "one-dark", "onelight": "one-light",
	"lotus": "kanagawa-lotus", "kanagawa-wave": "kanagawa", "rosepine": "rose-pine", "rosepine-dawn": "rose-pine-dawn",
	"rose-pine-main": "rose-pine", "dawn": "rose-pine-dawn",
}

// siblings pair each dark theme with its light one (both ways).
var siblings = map[string]string{}

func init() {
	for _, pr := range [][2]string{
		{"catppuccin", "catppuccin-latte"},
		{"tokyo-night", "tokyo-night-day"},
		{"gruvbox", "gruvbox-light"},
		{"one-dark", "one-light"},
		{"solarized", "solarized-light"},
		{"kanagawa", "kanagawa-lotus"},
		{"rose-pine", "rose-pine-dawn"},
	} {
		siblings[pr[0]], siblings[pr[1]] = pr[1], pr[0]
	}
}

// Canonical is a theme's name as Names has it: lowercased, spaces and _ as
// -, an alias resolved; ok is false for a name that is no theme.
func Canonical(name string) (string, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.NewReplacer(" ", "-", "_", "-").Replace(n)
	if a, ok := aliases[n]; ok {
		n = a
	}
	if _, ok := builtins[n]; ok {
		return n, true
	}
	return "", false
}

// IsLight reports a light theme (the light one of a pair).
func IsLight(name string) bool {
	switch name {
	case "catppuccin-latte", "tokyo-night-day", "gruvbox-light", "one-light", "solarized-light",
		"kanagawa-lotus", "rose-pine-dawn":
		return true
	}
	return false
}

// LightOf and DarkOf are a theme's light and dark siblings (itself when it
// has none, or is that one).
func LightOf(name string) string {
	if s, ok := siblings[name]; ok && !IsLight(name) {
		return s
	}
	return name
}

func DarkOf(name string) string {
	if s, ok := siblings[name]; ok && IsLight(name) {
		return s
	}
	return name
}

// Builtin is the palette of a built-in theme (Default's for another name).
func Builtin(name string) Palette {
	if n, ok := Canonical(name); ok {
		name = n
	} else {
		name = Default
	}
	cs := builtins[name]
	p := Palette{Name: name}
	for i, c := range p.colors() {
		*c = cs[i]
	}
	return p
}

// row makes a palette's colors from hex strings, "reset" and "ansi:N", in
// Tokens' order.
func row(vals ...string) []Color {
	if len(vals) != len(Tokens) {
		panic(fmt.Sprintf("theme: %d colors, want %d", len(vals), len(Tokens)))
	}
	out := make([]Color, len(vals))
	for i, v := range vals {
		switch {
		case v == "reset":
			out[i] = Reset
		case strings.HasPrefix(v, "ansi:"):
			var n int
			_, _ = fmt.Sscanf(v, "ansi:%d", &n)
			out[i] = ANSI(n)
		default:
			out[i] = Hex(v)
		}
	}
	return out
}

// builtins are the themes' colors, in Tokens' order: accent, panel_bg,
// sidebar_bg, active_row_bg, selection_bg, surface0, surface1, surface_dim,
// overlay0, overlay1, text, subtext0, mauve, green, yellow, red, blue, teal,
// peach. Each is its scheme's published palette (Catppuccin, Tokyo Night,
// Gruvbox, One, Solarized, Kanagawa, Rosé Pine, Dracula, Nord, Vesper).
var builtins = map[string][]Color{
	"catppuccin": row("#89b4fa", "#181825", "reset", "#1e1e2e", "#313244", "#313244", "#45475a", "#1e1e2e",
		"#6c7086", "#7f849c", "#cdd6f4", "#a6adc8", "#cba6f7", "#a6e3a1", "#f9e2af", "#f38ba8", "#89b4fa", "#94e2d5", "#fab387"),
	"catppuccin-latte": row("#1e66f5", "#eff1f5", "reset", "#e6e9ef", "#bdd0f5", "#ccd0da", "#bcc0cc", "#e6e9ef",
		"#9ca0b0", "#8c8fa1", "#4c4f69", "#6c6f85", "#8839ef", "#40a02b", "#df8e1d", "#d20f39", "#1e66f5", "#179299", "#fe640b"),
	// The terminal's own colors: ANSI blue, dark gray, gray…
	"terminal": row("ansi:4", "reset", "reset", "ansi:8", "ansi:8", "ansi:8", "ansi:8", "ansi:8",
		"ansi:7", "ansi:15", "reset", "ansi:7", "ansi:5", "ansi:2", "ansi:3", "ansi:9", "ansi:4", "ansi:6", "ansi:3"),
	"tokyo-night": row("#7aa2f7", "#1a1b26", "reset", "#232636", "#2d3650", "#24283b", "#414868", "#1a1b26",
		"#565f89", "#697196", "#c0caf5", "#a9b1d6", "#bb9af7", "#9ece6a", "#e0af68", "#f7768e", "#7aa2f7", "#7dcfff", "#ff9e64"),
	"tokyo-night-day": row("#2e7de9", "#e1e2e7", "reset", "#d2d3da", "#b6cae7", "#c4c8da", "#a8aecb", "#d2d3da",
		"#8990b3", "#68709a", "#3760bf", "#6172b0", "#7847bd", "#587539", "#8c6c3e", "#f52a65", "#2e7de9", "#118c74", "#b15c00"),
	"dracula": row("#bd93f9", "#21222c", "reset", "#282a36", "#44475a", "#343746", "#44475a", "#282a36",
		"#6272a4", "#7b88b8", "#f8f8f2", "#c9cad3", "#bd93f9", "#50fa7b", "#f1fa8c", "#ff5555", "#8be9fd", "#8be9fd", "#ffb86c"),
	"nord": row("#88c0d0", "#272c36", "reset", "#2e3440", "#434c5e", "#3b4252", "#434c5e", "#2e3440",
		"#4c566a", "#616e88", "#eceff4", "#d8dee9", "#b48ead", "#a3be8c", "#ebcb8b", "#bf616a", "#81a1c1", "#8fbcbb", "#d08770"),
	"gruvbox": row("#d79921", "#282828", "reset", "#323130", "#4b3f27", "#3c3836", "#504945", "#282828",
		"#928374", "#a89984", "#ebdbb2", "#d5c4a1", "#d3869b", "#b8bb26", "#fabd2f", "#fb4934", "#83a598", "#8ec07c", "#fe8019"),
	"gruvbox-light": row("#076678", "#fbf1c7", "reset", "#f2e5bc", "#ebdbb2", "#ebdbb2", "#d5c4a1", "#f2e5bc",
		"#928374", "#7c6f64", "#3c3836", "#504945", "#8f3f71", "#79740e", "#b57614", "#9d0006", "#076678", "#427b58", "#af3a03"),
	"one-dark": row("#61afef", "#21252b", "reset", "#2c313c", "#3e4451", "#3e4451", "#4b5263", "#282c34",
		"#5c6370", "#7f848e", "#abb2bf", "#9da5b4", "#c678dd", "#98c379", "#e5c07b", "#e06c75", "#61afef", "#56b6c2", "#d19a66"),
	"one-light": row("#4078f2", "#f0f0f1", "reset", "#ececed", "#d3dcf7", "#e5e5e6", "#d4d4d6", "#f0f0f1",
		"#a0a1a7", "#818387", "#383a42", "#696c77", "#a626a4", "#50a14f", "#c18401", "#e45649", "#4078f2", "#0184bc", "#986801"),
	"solarized": row("#268bd2", "#00212b", "reset", "#073642", "#0a4555", "#073642", "#0b4452", "#002b36",
		"#586e75", "#657b83", "#93a1a1", "#839496", "#6c71c4", "#859900", "#b58900", "#dc322f", "#268bd2", "#2aa198", "#cb4b16"),
	"solarized-light": row("#268bd2", "#eee8d5", "reset", "#eee8d5", "#e4ddc8", "#eee8d5", "#e4ddc8", "#fdf6e3",
		"#93a1a1", "#839496", "#586e75", "#657b83", "#6c71c4", "#859900", "#b58900", "#dc322f", "#268bd2", "#2aa198", "#cb4b16"),
	"kanagawa": row("#7e9cd8", "#16161d", "reset", "#2a2a37", "#223249", "#2a2a37", "#363646", "#1f1f28",
		"#54546d", "#727169", "#dcd7ba", "#c8c093", "#957fb8", "#98bb6c", "#e6c384", "#e46876", "#7e9cd8", "#7aa89f", "#ffa066"),
	"kanagawa-lotus": row("#4d699b", "#e5ddb0", "reset", "#e7dba0", "#c7d7e0", "#e7dba0", "#dcd5ac", "#f2ecbc",
		"#8a8980", "#716e61", "#545464", "#43436c", "#624c83", "#6f894e", "#77713f", "#c84053", "#4d699b", "#597b75", "#cc6d00"),
	"rose-pine": row("#c4a7e7", "#1f1d2e", "reset", "#21202e", "#403d52", "#26233a", "#403d52", "#191724",
		"#6e6a86", "#908caa", "#e0def4", "#908caa", "#c4a7e7", "#31748f", "#f6c177", "#eb6f92", "#9ccfd8", "#9ccfd8", "#ebbcba"),
	"rose-pine-dawn": row("#907aa9", "#fffaf3", "reset", "#f4ede8", "#dfdad9", "#f2e9e1", "#dfdad9", "#faf4ed",
		"#9893a5", "#797593", "#575279", "#797593", "#907aa9", "#286983", "#ea9d34", "#b4637a", "#56949f", "#56949f", "#d7827e"),
	"vesper": row("#ffc799", "#161616", "reset", "#1c1c1c", "#282828", "#232323", "#343434", "#101010",
		"#505050", "#8b8b8b", "#ffffff", "#a0a0a0", "#ffcfa8", "#99ffe4", "#ffc799", "#ff8080", "#99ffe4", "#99ffe4", "#ffc799"),
}
