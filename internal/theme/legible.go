package theme

import "math"

// Contrast is the WCAG contrast ratio of two 24-bit colors (1 to 21); 21
// when either is not 24-bit (the terminal decides).
func Contrast(a, b Color) float64 {
	if !a.IsRGB() || !b.IsRGB() {
		return 21
	}
	la, lb := relLuma(a), relLuma(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func relLuma(c Color) float64 {
	ch := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
}

var (
	black = Color{k: kindRGB}
	white = Color{k: kindRGB, R: 255, G: 255, B: 255}
)

// away is the far end from bg: black on a light background, white on a
// dark one.
func away(bg Color) Color {
	if relLuma(bg) > 0.18 {
		return black
	}
	return white
}

// readable moves *c toward the far end from the backgrounds until it has
// at least min contrast on each of them (or cannot get more).
func readable(c *Color, bgs []Color, min float64) {
	if !c.IsRGB() {
		return
	}
	worst := func() float64 {
		w := 21.0
		for _, b := range bgs {
			w = math.Min(w, Contrast(*c, b))
		}
		return w
	}
	end := away(bgs[0])
	for i := 0; i < 40 && worst() < min; i++ {
		*c = Blend(*c, end, 0.08)
	}
}

// apart moves the background *c away from the panel's until the two can be
// told apart (at least min contrast).
func apart(c *Color, panel Color, min float64) {
	if !c.IsRGB() || !panel.IsRGB() {
		return
	}
	end := away(panel) // toward the text's side: lighter on a dark panel
	for i := 0; i < 40 && Contrast(*c, panel) < min; i++ {
		*c = Blend(*c, end, 0.04)
	}
}

// legible makes a built-in theme read on every background i-mux draws it
// on (D68): the selection, fields and borders stand out from the panel,
// then text, secondary text, hints, the accent and the state colors get
// enough contrast on the backgrounds they are drawn on. A scheme's own
// colors stay where they already read.
func (p *Palette) legible() {
	if !p.PanelBg.IsRGB() || p.Classic {
		return
	}
	apart(&p.SelectionBg, p.PanelBg, 1.3)
	apart(&p.ActiveRowBg, p.PanelBg, 1.08)
	apart(&p.Surface0, p.PanelBg, 1.12)
	apart(&p.Surface1, p.PanelBg, 1.3)
	hover := HoverBg(*p)
	rows := []Color{p.PanelBg, p.SelectionBg, p.ActiveRowBg, hover, p.Surface0, p.Surface1}
	readable(&p.Text, rows, 4.5)
	readable(&p.Subtext0, []Color{p.PanelBg, p.ActiveRowBg, p.SelectionBg, p.Surface0}, 3)
	readable(&p.Overlay0, []Color{p.PanelBg, p.ActiveRowBg}, 2.2)
	readable(&p.Overlay1, []Color{p.PanelBg, p.Surface0}, 2.5)
	readable(&p.Accent, []Color{p.PanelBg, p.SelectionBg}, 3)
	for _, c := range []*Color{&p.Green, &p.Yellow, &p.Red, &p.Blue, &p.Teal, &p.Peach, &p.Mauve} {
		readable(c, []Color{p.PanelBg, p.ActiveRowBg}, 2.6)
	}
}

// need is a color drawn on a tint, and the contrast it must keep there.
type need struct {
	fg  Color
	min float64
}

// tintColor is base mixed with c, t of the way at most, weaker where it
// must be for the colors drawn on it to keep their contrast; ok is false
// when either is no 24-bit color.
func tintColor(p Palette, c Color, t float64, needs ...need) (Color, bool) {
	base := p.PanelBg
	if !base.IsRGB() {
		base = p.SurfaceDim
	}
	if !c.IsRGB() || !base.IsRGB() {
		return Color{}, false
	}
	for ; t > 0.06; t -= 0.02 {
		bg := Blend(base, c, t)
		ok := true
		for _, n := range needs {
			ok = ok && Contrast(n.fg, bg) >= n.min
		}
		if ok {
			return bg, true
		}
	}
	return Blend(base, c, 0.06), true
}

// tintFor is tintColor's background escape, or fallback.
func tintFor(p Palette, c Color, t float64, fallback string, needs ...need) string {
	if bg, ok := tintColor(p, c, t, needs...); ok {
		return bg.Bg()
	}
	return fallback
}
