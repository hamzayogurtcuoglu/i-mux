package sidebar

import (
	"strings"
	"unicode/utf8"
)

// ScrollBar is the right-edge column of a list of total rows of which view
// rows show from top: one drawn cell per shown row, the thumb sized and
// placed by what is shown. nil when everything fits.
func ScrollBar(total, top, view int) []string {
	if view <= 0 || total <= view {
		return nil
	}
	thumb := max(1, (view*view+total/2)/total)
	last := total - view
	top = min(max(top, 0), last)
	pos := (top*(view-thumb) + last/2) / last
	out := make([]string, view)
	for i := range out {
		out[i] = barTrack
		if i >= pos && i < pos+thumb {
			out[i] = barThumb
		}
	}
	return out
}

// ScrollTo is the top row that a click on row y (0-based) of the bar asks
// for: the top when y is the first row, the end when it is the last.
func ScrollTo(total, view, y int) int {
	if total <= view || view <= 1 {
		return 0
	}
	last := total - view
	return min(max((y*last+(view-1)/2)/(view-1), 0), last)
}

// WithBar draws bar's cells over the last of w columns of lines.
func WithBar(lines, bar []string, w int) []string {
	if bar == nil {
		return lines
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l
		if i < len(bar) {
			out[i] = Cut(l, 0, w-1) + bar[i]
		}
	}
	return out
}

// Clip cuts a drawn line (escape sequences kept) to w columns.
func Clip(s string, w int) string { return clip(s, w) }

// Cut keeps w columns of a drawn line from column from on, with its colors,
// padded with spaces to w: the pager's horizontal scrolling.
func Cut(s string, from, w int) string {
	var b strings.Builder
	col, used := 0, 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b { // escape sequences are kept: they set colors
			j := escapeEnd(s, i)
			b.WriteString(s[i:j])
			i = j
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		cw := runeWidth(r)
		switch {
		case col < from:
			if col+cw > from {
				b.WriteString(strings.Repeat(" ", col+cw-from)) // half of a wide rune
				used += col + cw - from
			}
		case used+cw > w:
			return b.String() + strings.Repeat(" ", max(w-used, 0)) + reset
		default:
			b.WriteString(s[i : i+size])
			used += cw
		}
		col += cw
		i += size
	}
	return b.String() + strings.Repeat(" ", max(w-used, 0))
}

// escapeEnd is the end of the escape sequence at s[i].
func escapeEnd(s string, i int) int {
	j := i + 1
	switch {
	case j < len(s) && s[j] == '[':
		j++
		for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
			j++
		}
		j++
	case j < len(s) && s[j] == ']':
		for j < len(s) && s[j] != 0x07 && (s[j] != 0x1b || j+1 >= len(s) || s[j+1] != '\\') {
			j++
		}
		if j < len(s) && s[j] == 0x1b {
			j++
		}
		j++
	default:
		j++
	}
	return min(j, len(s))
}
