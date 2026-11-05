// Package clean strips CLI wrapper tags from message text and truncates
// text safely on rune boundaries.
//
// The tag list is data (Rules), not code, so new wrappers can be added
// without touching parsers.
package clean

import (
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// Action says what to do with a wrapped block.
type Action uint8

// Actions.
const (
	Drop   Action = iota // remove the block entirely
	Notice               // replace the block with a one-line notice
	Unwrap               // keep the inner text, drop the tags
)

// Rule handles one wrapper tag.
type Rule struct {
	Tag    string
	Action Action
	Label  string // Notice: prefix of the one-line notice
}

// ClaudeRules are the wrappers Claude Code puts into user text (§7.1).
var ClaudeRules = []Rule{
	{Tag: "system-reminder", Action: Drop},
	{Tag: "ide_selection", Action: Drop},
	{Tag: "ide_opened_file", Action: Drop},
	{Tag: "ide_diagnostics", Action: Drop},
	{Tag: "system-note", Action: Drop},
	{Tag: "local-command-stdout", Action: Drop},
	{Tag: "local-command-stderr", Action: Drop},
	{Tag: "local-command-caveat", Action: Drop},
	{Tag: "command-message", Action: Drop},
	{Tag: "user-memory-input", Action: Unwrap},
	{Tag: "bash-input", Action: Unwrap},
	{Tag: "bash-stdout", Action: Drop},
	{Tag: "bash-stderr", Action: Drop},
	{Tag: "task-notification", Action: Notice, Label: "background task"},
}

// GenericRules apply to every agent's user text.
var GenericRules = []Rule{
	{Tag: "system-reminder", Action: Drop},
	{Tag: "environment_context", Action: Drop},
	{Tag: "user_instructions", Action: Drop},
}

var (
	reMu    sync.Mutex
	reCache = map[string]*regexp.Regexp{}
	anyTag  = regexp.MustCompile(`<[^>]+>`)
)

func tagRe(tag string) *regexp.Regexp {
	reMu.Lock()
	defer reMu.Unlock()
	if re, ok := reCache[tag]; ok {
		return re
	}
	q := regexp.QuoteMeta(tag)
	re := regexp.MustCompile(`(?s)<` + q + `(?:\s[^>]*)?>(.*?)</` + q + `>`)
	reCache[tag] = re
	return re
}

func init() {
	for _, r := range append(ClaudeRules, GenericRules...) {
		tagRe(r.Tag)
	}
	tagRe("summary")
	tagRe("status")
}

// Text applies rules to s and collapses the blank lines left behind.
func Text(s string, rules []Rule) string {
	if !strings.Contains(s, "<") {
		return strings.TrimSpace(s)
	}
	for _, r := range rules {
		if !strings.Contains(s, "<"+r.Tag) {
			continue
		}
		re := tagRe(r.Tag)
		switch r.Action {
		case Drop:
			s = re.ReplaceAllString(s, "")
		case Unwrap:
			s = re.ReplaceAllString(s, "$1")
		case Notice:
			s = re.ReplaceAllStringFunc(s, func(m string) string {
				inner := re.FindStringSubmatch(m)[1]
				return "[" + r.Label + ": " + noticeText(inner) + "]"
			})
		}
	}
	return collapseBlank(strings.TrimSpace(s))
}

// noticeText picks the most useful short line from a wrapped block.
func noticeText(inner string) string {
	for _, tag := range []string{"summary", "status"} {
		if m := tagRe(tag).FindStringSubmatch(inner); m != nil && strings.TrimSpace(m[1]) != "" {
			return OneLine(m[1], 120)
		}
	}
	stripped := anyTag.ReplaceAllString(inner, " ")
	if t := OneLine(stripped, 120); t != "" {
		return t
	}
	return "notification"
}

var (
	cmdNameRe = regexp.MustCompile(`(?s)<command-name>\s*(.*?)\s*</command-name>`)
	cmdArgsRe = regexp.MustCompile(`(?s)<command-args>(.*?)</command-args>`)
)

// SlashCommand turns Claude's <command-name>/x</command-name>
// <command-args>y</command-args> into "/x y".
func SlashCommand(s string) (string, bool) {
	m := cmdNameRe.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	name := strings.TrimSpace(m[1])
	if name == "" {
		return "", false
	}
	if !strings.HasPrefix(name, "/") {
		name = "/" + name
	}
	if a := cmdArgsRe.FindStringSubmatch(s); a != nil {
		if args := strings.TrimSpace(a[1]); args != "" {
			return name + " " + args, true
		}
	}
	return name, true
}

var blankRe = regexp.MustCompile(`\n[ \t]*\n(?:[ \t]*\n)+`)

func collapseBlank(s string) string { return blankRe.ReplaceAllString(s, "\n\n") }

// Runes returns the number of runes in s.
func Runes(s string) int { return utf8.RuneCountInString(s) }

// Omitted renders the omission marker.
func Omitted(n int) string { return "[… " + Thousands(n) + " chars omitted …]" }

// Thousands formats n with comma separators (1234 → "1,234").
func Thousands(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// Truncate shortens s to at most max runes by cutting out the middle: it
// keeps 60% from the head and 40% from the tail and inserts an omission
// marker. Cuts happen on rune boundaries.
func Truncate(s string, max int) string {
	n := Runes(s)
	if n <= max {
		return s
	}
	if max <= 0 {
		return ""
	}
	marker := "\n" + Omitted(n) + "\n"
	room := max - Runes(marker)
	if room < 2 {
		return firstRunes(s, max)
	}
	head := room * 6 / 10
	tail := room - head
	return firstRunes(s, head) + marker + lastRunes(s, tail)
}

// TruncateHead keeps the newest (last) part of s within max runes.
func TruncateHead(s string, max int) string {
	n := Runes(s)
	if n <= max {
		return s
	}
	marker := Omitted(n) + "\n"
	room := max - Runes(marker)
	if room < 1 {
		return lastRunes(s, max)
	}
	return marker + lastRunes(s, room)
}

// OneLine collapses whitespace and returns at most max runes, with "…" when cut.
func OneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if Runes(s) <= max {
		return s
	}
	if max <= 1 {
		return firstRunes(s, max)
	}
	return strings.TrimRight(firstRunes(s, max-1), " ") + "…"
}

func firstRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	i := 0
	for pos := range s {
		if i == n {
			return s[:pos]
		}
		i++
	}
	return s
}

func lastRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	total := Runes(s)
	if n >= total {
		return s
	}
	skip := total - n
	i := 0
	for pos := range s {
		if i == skip {
			return s[pos:]
		}
		i++
	}
	return ""
}
