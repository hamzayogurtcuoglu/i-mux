package approve

import (
	"regexp"
	"strconv"
	"strings"
)

// Key is the key that answers p's permission prompt for agent with the plain
// yes, this once ("" and ok false when i-mux leaves the prompt to the user):
// the number Claude Code and Copilot list, the key Codex's own row shows,
// Antigravity's confirm key, OpenCode's Enter on its highlighted button (the
// watcher checks the highlight with HighlightedOnce first). The answer is
// never "Yes, and don't ask again" or a switch of mode, which would write a
// rule into the agent's own settings or widen the whole session: only the
// first, plain yes is taken.
func Key(agent string, p *Prompt) (string, bool) {
	switch agent {
	case "claude":
		return numbered(p, claudeQ, yesExact, "1")
	case "copilot":
		return numbered(p, copilotQ, yesPlain, "1")
	case "codex":
		if !numberedShape(p, codexQ, codexYes) {
			return "", false
		}
		// The key the row itself shows, its number, else Enter on the
		// highlighted row.
		if k := p.Options[0].Key; k != "" {
			return k, true
		}
		if p.Options[0].N > 0 {
			return strconv.Itoa(p.Options[0].N), true
		}
		if p.Options[0].Selected {
			return "Enter", true
		}
		return "", false
	case "antigravity":
		return numbered(p, agyQ, yesPlain, "y")
	case "opencode":
		if p == nil || !p.OpenCode || !asked(p, openCodeQ) {
			return "", false
		}
		return "Enter", true
	}
	return "", false
}

// numbered is the key for a numbered prompt whose first option is the plain
// yes and whose last is a no, with its question above.
func numbered(p *Prompt, q *regexp.Regexp, yes func(string) bool, key string) (string, bool) {
	if !numberedShape(p, q, yes) {
		return "", false
	}
	return key, true
}

// numberedShape reports whether p is a numbered permission prompt that agent
// answers: options 1..N, the first the plain yes, the last a no, and the
// question above them.
func numberedShape(p *Prompt, q *regexp.Regexp, yes func(string) bool) bool {
	if p == nil || p.OpenCode || len(p.Options) < 2 {
		return false
	}
	first, last := p.Options[0], p.Options[len(p.Options)-1]
	if first.N != 1 || !yes(first.Label) {
		return false
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(last.Label)), "no") {
		return false
	}
	return asked(p, q)
}

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

// The question each CLI asks, and the first option that is its plain yes.
var (
	// Claude Code: "Do you want to proceed?", "Do you want to make this edit
	// to a.go?", "Claude requested permissions to use WebFetch, but you
	// haven't granted it yet.", "This command requires approval".
	claudeQ = regexp.MustCompile(`(?i)\bdo you want (to|me to)\b|\brequested permissions?\b|\brequires (your )?approval\b|^\s*allow\b[^?]*\?`)
	// Copilot CLI: "Do you want to run this command?", "Do you want to make
	// this edit to a.go?", and the organization policy's wording.
	copilotQ = regexp.MustCompile(`(?i)\bdo you want (to|me to)\b|\bwould you like\b|\borganization policy\b`)
	// Codex: "Would you like to run the following command?", "… make the
	// following edits?", "… grant these permissions?", "Do you want to
	// approve network access to …?".
	codexQ = regexp.MustCompile(`(?i)\bwould you like to (run|make|grant|send)\b|\bdo you want to approve\b`)
	// Antigravity CLI: "Do you want to proceed?", "Allow creation of this
	// file?", "requires your approval".
	agyQ = regexp.MustCompile(`(?i)\b(do you want|would you like)\b[^?]*\?|^\s*allow\b[^?]*\?|\brequires (your )?(approval|confirmation)\b`)
	// OpenCode: the "Permission required" header above its buttons.
	openCodeQ = regexp.MustCompile(`(?i)\bpermission required\b`)
)

// yesExact is Claude Code's first option: "Yes", no more.
func yesExact(l string) bool { return strings.EqualFold(strings.TrimSpace(l), "yes") }

// yesPlain is a first option that starts with the word yes: "Yes",
// "Yes, proceed", "Yes, allow creation".
func yesPlain(l string) bool {
	l = strings.TrimSpace(l)
	return len(l) >= 3 && strings.EqualFold(l[:3], "yes") && (len(l) == 3 || l[3] == ' ' || l[3] == ',')
}

// codexYes is Codex's narrowest yes: "Yes, proceed", "Yes, just this once",
// "Yes, grant these permissions for this turn" — never the ones that save a
// rule or widen the session.
func codexYes(l string) bool {
	l = strings.TrimSpace(l)
	if strings.EqualFold(l, "yes") {
		return true
	}
	for _, p := range []string{"yes, proceed", "yes, just this once", "yes, grant these permissions"} {
		if len(l) >= len(p) && strings.EqualFold(l[:len(p)], p) {
			return true
		}
	}
	return false
}
