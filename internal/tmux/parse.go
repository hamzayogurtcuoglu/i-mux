package tmux

import (
	"fmt"
	"strings"
)

// ParseCommandLine splits a tmux command string as imux writes them (menu
// items, bindings) into commands: words separated by spaces, ' ; ' between
// commands, single-quoted words taken as they are, double-quoted words with
// \ and \" escapes, and elsewhere a backslash escaping the next character
// (which joins a quote into a single-quoted word).
func ParseCommandLine(s string) ([]Cmd, error) {
	var cmds []Cmd
	var cur Cmd
	var word strings.Builder
	inWord := false
	end := func() {
		if inWord {
			cur = append(cur, word.String())
			word.Reset()
			inWord = false
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ' ' || c == '\t':
			end()
		case c == ';' && !inWord:
			if len(cur) > 0 {
				cmds = append(cmds, cur)
			}
			cur = nil
		case c == '\'':
			inWord = true
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				return nil, fmt.Errorf("tmux: unterminated quote in %q", s)
			}
			word.WriteString(s[i+1 : i+1+j])
			i += j + 1
		case c == '"':
			inWord = true
			i++
			for ; i < len(s) && s[i] != '"'; i++ {
				if s[i] == '\\' && i+1 < len(s) {
					i++
				}
				word.WriteByte(s[i])
			}
			if i >= len(s) {
				return nil, fmt.Errorf("tmux: unterminated quote in %q", s)
			}
		case c == '\\' && i+1 < len(s):
			inWord = true
			i++
			word.WriteByte(s[i])
		default:
			inWord = true
			word.WriteByte(c)
		}
	}
	end()
	if len(cur) > 0 {
		cmds = append(cmds, cur)
	}
	return cmds, nil
}
