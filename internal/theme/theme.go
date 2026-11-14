// Package theme is i-mux's color palette (ARCHITECTURE "Themes", D68): 19
// named colors that every part of the UI draws with, 18 built-in themes,
// the user's overrides, and light or dark following the terminal.
package theme

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
)

type kind uint8

const (
	kindRGB kind = iota
	kindANSI
	kindReset
	kind256
)

// Color is a 24-bit color, one of the 16 ANSI colors (the terminal's own),
// one of the 256 colors (the i-mux theme), or reset (the terminal's default).
type Color struct {
	k       kind
	R, G, B uint8
	ANSI    int // 0-15 with kindANSI, 0-255 with kind256
}

// Idx is 256-color n.
func Idx(n int) Color { return Color{k: kind256, ANSI: n} }

// Reset is the terminal's default color.
var Reset = Color{k: kindReset}

// Hex is the color #rrggbb (#rgb); it panics on anything else (built-ins).
func Hex(s string) Color {
	c, err := ParseColor(s)
	if err != nil || c.k != kindRGB {
		panic("theme: bad hex color " + s)
	}
	return c
}

// ANSI is ANSI color n (0-15).
func ANSI(n int) Color { return Color{k: kindANSI, ANSI: n} }

// IsReset reports the terminal's default color.
func (c Color) IsReset() bool { return c.k == kindReset }

// IsRGB reports a 24-bit color.
func (c Color) IsRGB() bool { return c.k == kindRGB }

// Fg is the escape that makes c the text color.
func (c Color) Fg() string {
	switch c.k {
	case kindRGB:
		return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", c.R, c.G, c.B)
	case kindANSI:
		if c.ANSI >= 8 {
			return "\x1b[" + strconv.Itoa(90+c.ANSI-8) + "m"
		}
		return "\x1b[" + strconv.Itoa(30+c.ANSI) + "m"
	case kind256:
		return "\x1b[38;5;" + strconv.Itoa(c.ANSI) + "m"
	}
	return "\x1b[39m"
}

// Bg is the escape that makes c the background.
func (c Color) Bg() string {
	switch c.k {
	case kindRGB:
		return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", c.R, c.G, c.B)
	case kindANSI:
		if c.ANSI >= 8 {
			return "\x1b[" + strconv.Itoa(100+c.ANSI-8) + "m"
		}
		return "\x1b[" + strconv.Itoa(40+c.ANSI) + "m"
	case kind256:
		return "\x1b[48;5;" + strconv.Itoa(c.ANSI) + "m"
	}
	return "\x1b[49m"
}

// tmuxANSI are tmux's names of the 16 ANSI colors.
var tmuxANSI = []string{
	"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
	"brightblack", "brightred", "brightgreen", "brightyellow", "brightblue", "brightmagenta", "brightcyan", "brightwhite",
}

// Tmux is c in a tmux style: "#rrggbb", an ANSI color's name, or "default".
func (c Color) Tmux() string {
	switch c.k {
	case kindRGB:
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	case kindANSI:
		return tmuxANSI[c.ANSI]
	case kind256:
		return "colour" + strconv.Itoa(c.ANSI) //nolint:misspell // tmux's spelling
	}
	return "default"
}

// Luma is c's brightness, 0 to 255000 (Rec. 601: 299 R + 587 G + 114 B);
// -1 for a color that is not 24-bit.
func (c Color) Luma() int {
	if c.k != kindRGB {
		return -1
	}
	return int(c.R)*299 + int(c.G)*587 + int(c.B)*114
}

// Light reports a light 24-bit color (luma at least half).
func (c Color) Light() bool { return c.Luma() >= 128000 }

// Blend is a mixed with b, t of the way to b (0..1). Only 24-bit colors
// mix; otherwise it is a.
func Blend(a, b Color, t float64) Color {
	if a.k != kindRGB || b.k != kindRGB {
		return a
	}
	mix := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5) }
	return Color{k: kindRGB, R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B)}
}

// ansiNames are the color names a config may use: ANSI's, with the names
// of their bright forms.
var ansiNames = map[string]int{
	"black": 0, "red": 1, "green": 2, "yellow": 3, "blue": 4, "magenta": 5, "cyan": 6, "white": 7,
	"gray": 7, "grey": 7, "darkgray": 8, "darkgrey": 8, "brightblack": 8,
	"lightred": 9, "brightred": 9, "lightgreen": 10, "brightgreen": 10, "lightyellow": 11, "brightyellow": 11,
	"lightblue": 12, "brightblue": 12, "lightmagenta": 13, "brightmagenta": 13, "lightcyan": 14, "brightcyan": 14,
	"brightwhite": 15,
}

// ParseColor reads a configured color: "#rgb", "#rrggbb", "rgb(r,g,b)",
// an ANSI name, or "reset" / "default" / "none" / "transparent".
func ParseColor(s string) (Color, error) {
	t := strings.ToLower(strings.TrimSpace(s))
	switch t {
	case "reset", "default", "none", "transparent":
		return Reset, nil
	}
	if n, ok := ansiNames[strings.NewReplacer("_", "", "-", "", " ", "").Replace(t)]; ok {
		return ANSI(n), nil
	}
	bad := fmt.Errorf("not a color: %q (#rrggbb, #rgb, rgb(r,g,b), a name such as blue, or reset)", s)
	if strings.HasPrefix(t, "#") {
		h := t[1:]
		if len(h) == 3 {
			h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
		}
		if len(h) != 6 {
			return Color{}, bad
		}
		v, err := strconv.ParseUint(h, 16, 32)
		if err != nil {
			return Color{}, bad
		}
		return Color{k: kindRGB, R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v)}, nil
	}
	if inner, ok := strings.CutPrefix(t, "rgb("); ok {
		inner, ok = strings.CutSuffix(inner, ")")
		parts := strings.Split(inner, ",")
		if !ok || len(parts) != 3 {
			return Color{}, bad
		}
		var v [3]uint8
		for i, p := range parts {
			n, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil || n < 0 || n > 255 {
				return Color{}, bad
			}
			v[i] = uint8(n)
		}
		return Color{k: kindRGB, R: v[0], G: v[1], B: v[2]}, nil
	}
	return Color{}, bad
}

// ParseOSCColor reads a terminal's reply to an OSC 10/11 query, "rgb:RRRR/GGGG/BBBB"
// (1 to 4 hex digits each), into a 24-bit color.
func ParseOSCColor(s string) (Color, bool) {
	body, ok := strings.CutPrefix(strings.TrimSpace(s), "rgb:")
	if !ok {
		return Color{}, false
	}
	parts := strings.Split(body, "/")
	if len(parts) != 3 {
		return Color{}, false
	}
	var v [3]uint8
	for i, p := range parts {
		if len(p) < 1 || len(p) > 4 {
			return Color{}, false
		}
		n, err := strconv.ParseUint(p, 16, 16)
		if err != nil {
			return Color{}, false
		}
		maxV := uint64(1)<<(4*len(p)) - 1
		v[i] = uint8((n*255 + maxV/2) / maxV)
	}
	return Color{k: kindRGB, R: v[0], G: v[1], B: v[2]}, true
}

// Appearances of the terminal (the global option @imux_appearance).
const (
	AppearanceLight = "light"
	AppearanceDark  = "dark"
)

// AppearanceOf is a terminal background's appearance.
func AppearanceOf(bg Color) string {
	if bg.Light() {
		return AppearanceLight
	}
	return AppearanceDark
}

// --- the current palette ----------------------------------------------------

var (
	mu       sync.Mutex
	current  = Builtin(Default)
	watchers []func(Palette)
)

// Current is the palette in use.
func Current() Palette {
	mu.Lock()
	defer mu.Unlock()
	return current
}

// Watch runs f with the palette now and each time it changes: a package
// keeps its escapes in step this way.
func Watch(f func(Palette)) {
	mu.Lock()
	watchers = append(watchers, f)
	p := current
	mu.Unlock()
	f(p)
}

// Set makes p the palette in use; it reports whether it changed.
func Set(p Palette) bool {
	mu.Lock()
	if p == current {
		mu.Unlock()
		return false
	}
	current = p
	ws := append([]func(Palette){}, watchers...)
	mu.Unlock()
	for _, f := range ws {
		f(p)
	}
	return true
}
