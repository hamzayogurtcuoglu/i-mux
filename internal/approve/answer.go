package approve

import (
	"regexp"
	"strconv"
	"strings"
)

// Answer is the key that answers p's permission prompt for agent, and the
// choice it takes (its label): the first choice whose text holds one of
// words (case-insensitive), in the order the choices are listed — on every
// CLI today that is the plain yes, choice 1 (D113). The key pressed is the
// choice's own: the key its row shows, else its number, else Enter on the
// highlighted row. OpenCode's buttons have no keys, so Enter, and the
// watcher checks with the screen's colors that the choice is the
// highlighted one; Antigravity answers with its confirm key on the first
// choice.
func Answer(agent string, p *Prompt, words []string) (key, label string, ok bool) {
	if p == nil {
		return "", "", false
	}
	ws := normalizeWords(words)
	i, found := matchIndex(p.Options, ws)
	if !found {
		return "", "", false
	}
	o := p.Options[i]
	if p.OpenCode {
		// Its prompt is the buttons; Enter takes the highlighted one.
		if agent != "opencode" || !asked(p, openCodeQ) {
			return "", "", false
		}
		return "Enter", o.Label, true
	}
	if agent == "antigravity" && i == 0 {
		// The CLI's own confirm key, active during a confirmation prompt.
		return "y", o.Label, true
	}
	if k := o.Key; k != "" {
		return k, o.Label, true
	}
	if o.N > 0 {
		return strconv.Itoa(o.N), o.Label, true
	}
	if o.Selected {
		return "Enter", o.Label, true
	}
	return "", "", false
}

// openCodeQ matches the "Permission required" header above its buttons.
var openCodeQ = regexp.MustCompile(`(?i)\bpermission required\b`)

// asked reports whether one of the lines above the choices is the prompt's
// question, matching q.
func asked(p *Prompt, q *regexp.Regexp) bool {
	for _, l := range p.Above {
		if q.MatchString(l) {
			return true
		}
	}
	return false
}

// normalizeWords returns the words as they are matched: trimmed and lower-case.
func normalizeWords(in []string) []string {
	var out []string
	for _, w := range in {
		if w = strings.ToLower(strings.TrimSpace(w)); w != "" {
			out = append(out, w)
		}
	}
	return out
}

// matchIndex is the first choice whose text holds one of words.
func matchIndex(opts []Option, words []string) (int, bool) {
	if len(words) == 0 {
		return 0, false
	}
	for i, o := range opts {
		l := strings.ToLower(o.Label)
		for _, w := range words {
			if strings.Contains(l, w) {
				return i, true
			}
		}
	}
	return 0, false
}
