// Package diffview draws a file's changes the way an editor does (ARCHITECTURE
// D26, §9.3 "Sidebar v6"): the whole file with line numbers, removed lines on
// red and added ones on green, side by side when there is room.
package diffview

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// Kind of a row.
const (
	Same    = ' '
	Removed = '-'
	Added   = '+'
)

// Line is one line of either side of the diff.
type Line struct {
	Kind  byte
	OldNo int // 0 when the line is not in the old file
	NewNo int // 0 when the line is not in the new file
	Text  string
}

// Parse reads a unified diff (git diff with a large -U). Header lines are
// skipped; "\ No newline at end of file" too.
func Parse(unified string) []Line {
	var out []Line
	oldNo, newNo := 0, 0
	inHunk := false
	for _, l := range strings.Split(strings.TrimRight(unified, "\n"), "\n") {
		if strings.HasPrefix(l, "@@") {
			inHunk = true
			// @@ -a,b +c,d @@
			f := strings.Fields(l)
			if len(f) >= 3 {
				oldNo = startOf(f[1]) - 1
				newNo = startOf(f[2]) - 1
			}
			continue
		}
		if !inHunk {
			continue
		}
		if strings.HasPrefix(l, `\`) {
			continue
		}
		switch {
		case strings.HasPrefix(l, "-"):
			oldNo++
			out = append(out, Line{Kind: Removed, OldNo: oldNo, Text: l[1:]})
		case strings.HasPrefix(l, "+"):
			newNo++
			out = append(out, Line{Kind: Added, NewNo: newNo, Text: l[1:]})
		default:
			oldNo++
			newNo++
			text := l
			if len(text) > 0 {
				text = text[1:]
			}
			out = append(out, Line{Kind: Same, OldNo: oldNo, NewNo: newNo, Text: text})
		}
	}
	return out
}

// startOf reads "-12,4" or "+7" as 12 or 7.
func startOf(s string) int {
	s = strings.TrimLeft(s, "-+")
	if i := strings.IndexByte(s, ','); i >= 0 {
		s = s[:i]
	}
	n, _ := strconv.Atoi(s)
	if n == 0 {
		return 1
	}
	return n
}

// AllAdded is a new file: every line added.
func AllAdded(content string) []Line {
	var out []Line
	content = strings.TrimSuffix(content, "\n")
	if content == "" {
		return nil
	}
	for i, l := range strings.Split(content, "\n") {
		out = append(out, Line{Kind: Added, NewNo: i + 1, Text: l})
	}
	return out
}

// Stats counts added and removed lines.
func Stats(lines []Line) (added, removed int) {
	for _, l := range lines {
		switch l.Kind {
		case Added:
			added++
		case Removed:
			removed++
		}
	}
	return
}

const (
	reset   = "\x1b[0m"
	dim     = "\x1b[38;5;243m"
	redBg   = "\x1b[48;5;52m"
	greenBg = "\x1b[48;5;22m"
	redFg   = "\x1b[38;5;210m"
	greenFg = "\x1b[38;5;114m"
	emptyBg = "\x1b[48;5;235m"
)

// clean expands tabs and drops control characters.
func clean(s string) string {
	s = strings.ReplaceAll(s, "\t", "    ")
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// fit cuts or pads s to w columns.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	n := utf8.RuneCountInString(s)
	if n > w {
		r := []rune(s)
		return string(r[:w-1]) + "›"
	}
	return s + strings.Repeat(" ", w-n)
}

func num(n, w int) string {
	if n == 0 {
		return strings.Repeat(" ", w)
	}
	s := strconv.Itoa(n)
	return strings.Repeat(" ", max(w-len(s), 0)) + s
}

// Render draws the diff for a terminal of width columns: side by side when
// side is true, else one column with both line numbers. It returns the lines
// and the index of the first changed one (-1 when nothing changed).
func Render(lines []Line, width int, side bool) ([]string, int) {
	maxNo := 1
	for _, l := range lines {
		maxNo = max(maxNo, l.OldNo, l.NewNo)
	}
	nw := len(strconv.Itoa(maxNo))
	first := -1
	var out []string
	if !side {
		tw := width - 2*nw - 5
		for _, l := range lines {
			if l.Kind != Same && first < 0 {
				first = len(out)
			}
			out = append(out, oneColumn(l, nw, tw))
		}
		return out, first
	}
	half := (width - 1) / 2
	tw := half - nw - 3
	// Pair each run of removed lines with the run of added lines after it.
	for i := 0; i < len(lines); {
		if lines[i].Kind == Same {
			l := lines[i]
			out = append(out, sideCell(&l, nw, tw, false)+dim+"│"+reset+sideCell(&l, nw, tw, true))
			i++
			continue
		}
		var rem, add []Line
		for i < len(lines) && lines[i].Kind == Removed {
			rem = append(rem, lines[i])
			i++
		}
		for i < len(lines) && lines[i].Kind == Added {
			add = append(add, lines[i])
			i++
		}
		if first < 0 {
			first = len(out)
		}
		for k := 0; k < max(len(rem), len(add)); k++ {
			var left, right *Line
			if k < len(rem) {
				left = &rem[k]
			}
			if k < len(add) {
				right = &add[k]
			}
			out = append(out, sideCell(left, nw, tw, false)+dim+"│"+reset+sideCell(right, nw, tw, true))
		}
	}
	return out, first
}

// sideCell is one half of a side-by-side row: the old side or the new one.
func sideCell(l *Line, nw, tw int, newSide bool) string {
	if l == nil {
		return emptyBg + strings.Repeat(" ", nw+3+tw) + reset
	}
	no := l.OldNo
	if newSide {
		no = l.NewNo
	}
	text := fit(clean(l.Text), tw)
	switch {
	case l.Kind == Removed && !newSide:
		return redBg + dim + num(no, nw) + " " + redFg + "-" + reset + redBg + " " + text + reset
	case l.Kind == Added && newSide:
		return greenBg + dim + num(no, nw) + " " + greenFg + "+" + reset + greenBg + " " + text + reset
	}
	return dim + num(no, nw) + reset + "   " + text
}

// oneColumn is "  12  13 │ text" with the change colored.
func oneColumn(l Line, nw, tw int) string {
	text := fit(clean(l.Text), tw)
	nums := dim + num(l.OldNo, nw) + " " + num(l.NewNo, nw) + reset
	switch l.Kind {
	case Removed:
		return nums + " " + redFg + "-" + reset + redBg + " " + text + reset
	case Added:
		return nums + " " + greenFg + "+" + reset + greenBg + " " + text + reset
	}
	return nums + "   " + text
}
