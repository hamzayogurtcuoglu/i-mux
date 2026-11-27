package approve

import (
	"strings"
	"unicode/utf8"
)

// OpenCode's permission prompt is not a numbered list but a row of buttons
// (D112): "Allow once", "Allow always" (only when it can save a rule) and
// "Reject", chosen with ← → and Enter. There is no key for the first button,
// so i-mux presses Enter — and only when it can see, in a capture with the
// screen's colors, that "Allow once" is the highlighted button.

// openCodeLines is the buttons among lines[from:to], the last lines of the
// screen: the prompt they build, and the index of the first line that holds
// a button (nil when they are not there). Nothing may be on those lines but
// the buttons and key hints.
func openCodeLines(lines []string, from, to int) (*Prompt, int) {
	first := -1
	var opts []Option
	for i := from; i < to; i++ {
		l := strings.Trim(lines[i], " \t│")
		if l == "" {
			continue
		}
		labels := openCodeLabels(l)
		if len(labels) == 0 {
			// Between or below the buttons: a key hint or nothing.
			if first < 0 || hint(lines[i]) {
				continue
			}
			return nil, 0
		}
		if !openCodeRow(l) {
			return nil, 0 // something else on the row
		}
		if first < 0 {
			first = i
		}
		for _, label := range labels {
			opts = append(opts, Option{Label: label})
		}
	}
	if first < 0 || !hasLabel(opts, "Allow once") || !hasLabel(opts, "Reject") {
		return nil, 0
	}
	return &Prompt{Options: opts, OpenCode: true}, first
}

// openCodeLabels are the buttons a line holds, in the order OpenCode shows
// them: once, the always it may offer, reject. The direct-mode footer spells
// the last two a little longer.
func openCodeLabels(l string) []string {
	var out []string
	if strings.Contains(l, "Allow once") {
		out = append(out, "Allow once")
	}
	if strings.Contains(l, "Allow always") || strings.Contains(l, "Always allow") {
		out = append(out, "Allow always")
	}
	if strings.Contains(l, "Reject") {
		out = append(out, "Reject")
	}
	return out
}

// openCodeLabel is the first button a line holds ("" for none), so that a
// line of buttons is never taken for a key hint.
func openCodeLabel(l string) (string, bool) {
	if ls := openCodeLabels(l); len(ls) > 0 {
		return ls[0], true
	}
	return "", false
}

// hasLabel reports whether the options hold one of OpenCode's buttons.
func hasLabel(opts []Option, label string) bool {
	for _, o := range opts {
		if o.Label == label {
			return true
		}
	}
	return false
}

// openCodeRow reports whether a button's row holds nothing but the buttons
// and key hints (" Allow once  Allow always  Reject   ⇆ select  enter
// confirm "): every word one of those.
func openCodeRow(l string) bool {
	for _, w := range strings.Fields(strings.ToLower(hintReplacer.Replace(strings.Trim(l, " \t│")))) {
		switch w {
		case "allow", "once", "always", "reject", "permission":
		default:
			if !hintWords[w] && !chord(w) {
				return false
			}
		}
	}
	return true
}

// Highlighted reports whether the want button ("Allow once", or whichever
// choice the words picked) is the highlighted one in capture, the screen read
// with its colors (`capture-pane -e`): the highlighted button's background
// differs from the others', which share one. When the buttons share a
// background, or there are only two, i-mux cannot tell, and the prompt is
// left to the user.
func Highlighted(capture, want string) bool {
	bg := map[string]string{}
	for _, line := range strings.Split(capture, "\n") {
		cells, text := cellBGs(line)
		if len(cells) == 0 {
			continue
		}
		// The bottom-most occurrence is the button's ("Reject" also covers
		// the footer's "Reject permission").
		for _, l := range []struct{ label, text string }{
			{"Allow once", "Allow once"},
			{"Allow always", "Allow always"},
			{"Allow always", "Always allow"},
			{"Reject", "Reject"},
		} {
			if i, ok := indexOfLabel(text, l.text); ok {
				bg[l.label] = cells[i]
			}
		}
	}
	w, ok := bg[want]
	if !ok || w == "" {
		return false
	}
	other, others := "", 0
	for l, b := range bg {
		if l == want {
			continue
		}
		if b == w {
			return false // another button shares the choice's background
		}
		if others == 0 {
			other = b
		} else if b != other {
			return false // the others do not share one background
		}
		others++
	}
	// With fewer than two others there is nothing to tell the highlighted
	// background from.
	return others >= 2
}

// indexOfLabel is where label starts in text, in runes, with the space
// before it so a button's background (padded by one cell) is the one read.
func indexOfLabel(text, label string) (int, bool) {
	b := strings.Index(text, label)
	if b < 0 {
		return 0, false
	}
	i := utf8.RuneCountInString(text[:b])
	if i > 0 {
		i--
	}
	return i, true
}

// cellBGs walks a captured line with its SGR sequences and returns the
// background in effect for each rune of the line's text, and that text.
// Only backgrounds are tracked: `48;5;N`, `48;2;R;G;B`, `49` (the default)
// and `0` (a reset).
func cellBGs(line string) ([]string, string) {
	var cells []string
	var text strings.Builder
	bg := ""
	i := 0
	for i < len(line) {
		if line[i] != 0x1b {
			r, sz := utf8.DecodeRuneInString(line[i:])
			if r == utf8.RuneError && sz == 1 {
				i++
				continue
			}
			text.WriteRune(r)
			cells = append(cells, bg)
			i += sz
			continue
		}
		j := i + 1
		if j < len(line) && line[j] == '[' { // CSI: to its final byte
			j++ // past the '['
			for j < len(line) && (line[j] < 0x40 || line[j] > 0x7e) {
				j++
			}
			if j < len(line) {
				if line[j] == 'm' {
					bg = sgrBG(line[i+2:j], bg)
				}
				j++
			}
		} else if j < len(line) && line[j] == ']' { // OSC: to BEL or ST
			for j < len(line) && line[j] != 0x07 {
				if line[j] == 0x1b && j+1 < len(line) && line[j+1] == '\\' {
					j++
					break
				}
				j++
			}
			if j < len(line) {
				j++
			}
		} else if j < len(line) {
			j++ // a two-byte escape
		}
		i = j
	}
	return cells, text.String()
}

// sgrBG is the background after one SGR's parameters. An empty parameter
// list is a reset.
func sgrBG(params string, cur string) string {
	if params == "" {
		return ""
	}
	f := strings.Split(params, ";")
	for i := 0; i < len(f); i++ {
		switch {
		case f[i] == "0" || f[i] == "":
			if f[i] == "0" {
				cur = ""
			}
		case f[i] == "49":
			cur = ""
		case f[i] == "48" && i+2 < len(f) && f[i+1] == "5":
			cur = "5:" + f[i+2]
			return cur
		case f[i] == "48" && i+4 < len(f) && f[i+1] == "2":
			cur = "2:" + f[i+2] + ":" + f[i+3] + ":" + f[i+4]
			return cur
		}
	}
	return cur
}
