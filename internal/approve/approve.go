// Package approve reads the agents' permission prompts on the screen and
// gives the key that answers each with its plain yes, this once (D112):
// the watcher sends it when Auto-approve is on (on_permission = "approve").
//
// A prompt is answered only when it is the last thing on the screen: the
// choices, then only blank lines, box borders and key hints, the question
// line above them. A transcript's numbered list, a question above the input
// box, a menu of the CLI's own and a plan whose first option is not the
// plain yes are all left to the user.
package approve

import (
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"
)

// Option is one choice of a prompt.
type Option struct {
	N        int    // its number on the screen (0 when the list is not numbered)
	Key      string // the key the row itself shows ("y"), ""
	Label    string
	Selected bool // the row the cursor is on
}

// Prompt is a permission prompt at the bottom of a screen.
type Prompt struct {
	Options []Option
	// Above holds the non-blank lines above the choices, bottom-up, at
	// most aboveLines of them: the question, and above it what the CLI
	// shows about what it wants to do.
	Above []string
	// OpenCode: the choices are its buttons, not a numbered list.
	OpenCode bool
}

// aboveLines is how many non-blank lines above the choices may hold the
// question.
const aboveLines = 12

// maxOptions bounds the choices read from one screen: their keys are digits.
const maxOptions = 9

// Find returns the permission prompt at the bottom of screen, nil when there
// is none. The choices end the screen: numbered rows in order, or OpenCode's
// buttons, with only blank lines, box borders and key hints below them.
func Find(screen string) *Prompt {
	lines := strings.Split(strings.TrimRight(screen, " \t\r\n"), "\n")
	i := len(lines)
	for i > 0 && (blank(lines[i-1]) || hint(lines[i-1])) {
		i--
	}
	if i == 0 {
		return nil
	}
	// Numbered rows, bottom-up: each row above the last is one number less.
	// A wrapped label's rest sits just below the row it belongs to, so it is
	// kept for the next row up.
	var rows []Option
	var wrap string
	for i > 0 && len(rows) < maxOptions {
		l := lines[i-1]
		if blank(l) {
			break
		}
		if o, ok := optionRow(l); ok {
			if len(rows) > 0 && o.N != rows[0].N-1 {
				break // not 1..N in order
			}
			if wrap != "" {
				o.Label += " " + wrap
				wrap = ""
			}
			rows = append([]Option{o}, rows...)
			i--
			continue
		}
		if continuation(l) {
			if wrap == "" {
				wrap = strings.TrimSpace(l)
			} else {
				wrap = strings.TrimSpace(l) + " " + wrap
			}
			i--
			continue
		}
		break
	}
	if len(rows) >= 2 && rows[0].N == 1 {
		return &Prompt{Options: rows, Above: above(lines[:i])}
	}
	// OpenCode's buttons: "Allow once", "Allow always", "Reject" in a row (a
	// column on a narrow terminal), the key hints beside or below them.
	if p, at := openCodeLines(lines, max(0, i-4), i); p != nil {
		p.Above = above(lines[:at])
		return p
	}
	return nil
}

// above returns the last aboveLines non-blank lines above the choices,
// bottom-up.
func above(lines []string) []string {
	var out []string
	for i := len(lines) - 1; i >= 0 && len(out) < aboveLines; i-- {
		if blank(lines[i]) {
			continue
		}
		out = append(out, strings.TrimRight(lines[i], " \t"))
	}
	return out
}

// optionRow parses one numbered choice ("❯ 1. Yes", "  3. No … (esc)").
func optionRow(l string) (Option, bool) {
	t := strings.TrimLeft(l, " \t│|")
	o := Option{}
	for _, m := range []string{"❯", "›", ">", "▸", "»"} {
		if strings.HasPrefix(t, m) {
			o.Selected = true
			t = strings.TrimLeft(t[len(m):], " ")
			break
		}
	}
	if t == "" || t[0] < '1' || t[0] > '9' {
		return Option{}, false
	}
	o.N = int(t[0] - '0')
	t = t[1:]
	if t == "" || (t[0] != '.' && t[0] != ')') {
		return Option{}, false
	}
	o.Label = strings.Trim(t[1:], " \t│|╯╮┘└─━═")
	// The key the row shows at its end, as Codex does ("Yes, proceed (y)").
	if m := keyHint.FindStringSubmatch(o.Label); m != nil && len(m[1]) == 1 {
		o.Key = m[1]
		o.Label = strings.TrimRight(o.Label[:len(o.Label)-len(m[0])], " ")
	}
	if o.Label == "" {
		return Option{}, false
	}
	return o, true
}

// continuation reports whether l is a wrapped label's rest: indented past
// where a choice's number starts, and not a key hint.
func continuation(l string) bool {
	t := strings.TrimLeft(l, " \t")
	if t == "" || hint(l) {
		return false
	}
	return runeLen(l)-runeLen(t) >= 2
}

// keyHint is the key a choice's row shows at its end, in parentheses.
var keyHint = regexp.MustCompile(`\(([^()]{1,8})\)[\s│|]*$`)

// Hash identifies the prompt's own block: what it asks and the choices. Two
// prompts with one hash are the same prompt, not two questions.
func (p *Prompt) Hash() uint64 {
	h := fnv.New64a()
	for _, l := range p.Above {
		_, _ = h.Write([]byte(l + "\x00"))
	}
	_, _ = h.Write([]byte("\x01"))
	for _, o := range p.Options {
		_, _ = h.Write([]byte(strconv.Itoa(o.N) + "\x02" + o.Label + "\x00"))
	}
	return h.Sum64()
}

func runeLen(s string) int { return len([]rune(s)) }

func blank(l string) bool {
	return strings.TrimSpace(strings.Trim(l, " \t│|╭╮╰╯─━═┌┐└┘")) == ""
}

// hintWords are what a line below the choices may say: how to answer them,
// and single letters as the key it names.
var hintWords = func() map[string]bool {
	m := map[string]bool{
		"esc": true, "enter": true, "return": true, "tab": true, "to": true,
		"cancel": true, "amend": true, "confirm": true, "select": true,
		"reject": true, "fullscreen": true, "minimize": true, "navigate": true,
		"press": true, "and": true, "or": true, "with": true, "keys": true,
		"for": true, "space": true, "edit": true,
	}
	for c := 'a'; c <= 'z'; c++ {
		m[string(c)] = true
	}
	return m
}()

// hintReplacer strips the punctuation a key hint's words sit in.
var hintReplacer = strings.NewReplacer("(", " ", ")", " ", ",", " ", ":", " ", ";", " ",
	"·", " ", "↑", " ", "↓", " ", "←", " ", "→", " ", "⇆", " ", "‹", " ", "›", " ")

// hint reports whether l is a key hint or a border: the only thing that may
// follow the choices on the screen. The input box, a spinner's hint or a
// status line has a word that is not one of these; a line that is only one
// of OpenCode's buttons is the button, not a hint.
func hint(l string) bool {
	if _, ok := openCodeLabel(strings.Trim(l, " \t│")); ok {
		return false
	}
	t := strings.Trim(l, " \t│|╭╮╰╯─━═┌┐└┘·-–—")
	if t == "" {
		return true // blank or a border only
	}
	for _, w := range strings.Fields(strings.ToLower(hintReplacer.Replace(t))) {
		if !hintWords[w] && !chord(w) {
			return false
		}
	}
	return true
}

// chord is a key with a modifier, "ctrl+g", "shift+tab".
func chord(w string) bool {
	for _, m := range []string{"ctrl", "alt", "shift", "cmd", "super"} {
		if strings.HasPrefix(w, m+"+") && len(w) > len(m)+1 {
			return true
		}
	}
	return false
}
