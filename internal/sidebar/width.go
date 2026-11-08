package sidebar

import (
	"strings"
	"unicode/utf8"
)

// wideRanges are the code points a terminal draws two columns wide: East
// Asian wide and full-width ones, and emoji presented as emoji (Unicode
// 15; what Terminal.app, iTerm2 and Ghostty do).
var wideRanges = [][2]rune{
	{0x1100, 0x115F},
	{0x231A, 0x231B},
	{0x2329, 0x232A},
	{0x23E9, 0x23EC},
	{0x23F0, 0x23F0},
	{0x23F3, 0x23F3},
	{0x25FD, 0x25FE},
	{0x2614, 0x2615},
	{0x2648, 0x2653},
	{0x267F, 0x267F},
	{0x2693, 0x2693},
	{0x26A1, 0x26A1},
	{0x26AA, 0x26AB},
	{0x26BD, 0x26BE},
	{0x26C4, 0x26C5},
	{0x26CE, 0x26CE},
	{0x26D4, 0x26D4},
	{0x26EA, 0x26EA},
	{0x26F2, 0x26F3},
	{0x26F5, 0x26F5},
	{0x26FA, 0x26FA},
	{0x26FD, 0x26FD},
	{0x2705, 0x2705},
	{0x270A, 0x270B},
	{0x2728, 0x2728},
	{0x274C, 0x274C},
	{0x274E, 0x274E},
	{0x2753, 0x2755},
	{0x2757, 0x2757},
	{0x2795, 0x2797},
	{0x27B0, 0x27B0},
	{0x27BF, 0x27BF},
	{0x2B1B, 0x2B1C},
	{0x2B50, 0x2B50},
	{0x2B55, 0x2B55},
	{0x2E80, 0x303E},
	{0x3041, 0x33FF},
	{0x3400, 0x4DBF},
	{0x4E00, 0x9FFF},
	{0xA000, 0xA4CF},
	{0xA960, 0xA97F},
	{0xAC00, 0xD7A3},
	{0xF900, 0xFAFF},
	{0xFE10, 0xFE19},
	{0xFE30, 0xFE6F},
	{0xFF00, 0xFF60},
	{0xFFE0, 0xFFE6},
	{0x1F000, 0x1FAFF},
	{0x20000, 0x3FFFD},
}

// runeWidth is how many columns r takes: 0 for combining marks, joiners
// and variation selectors, 2 for wide ones, else 1.
func runeWidth(r rune) int {
	switch {
	case r == 0x200D || r >= 0xFE00 && r <= 0xFE0F || r >= 0x0300 && r <= 0x036F || r >= 0x1F3FB && r <= 0x1F3FF:
		return 0
	case r < 0x1100:
		return 1
	}
	for _, w := range wideRanges {
		if r < w[0] {
			return 1
		}
		if r <= w[1] {
			return 2
		}
	}
	return 1
}

// DisplayWidth counts the terminal columns of plain text.
func DisplayWidth(s string) int {
	n := 0
	for _, r := range s {
		n += runeWidth(r)
	}
	return n
}

// clip cuts a drawn line (escape sequences kept) to w columns, so that no
// text can wrap and push the rows below (a ✨ in a commit subject did).
func clip(s string, w int) string {
	if w <= 0 {
		return s
	}
	var b strings.Builder
	used := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b { // copy an escape sequence whole
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
			j = min(j, len(s))
			b.WriteString(s[i:j])
			i = j
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		cw := runeWidth(r)
		if used+cw > w {
			b.WriteString(strings.Repeat(" ", w-used) + reset)
			return b.String()
		}
		b.WriteString(s[i : i+size])
		used += cw
		i += size
	}
	return b.String()
}
