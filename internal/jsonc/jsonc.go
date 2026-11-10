// Package jsonc reads and changes one string in a JSON file that may hold
// comments and trailing commas (Windows Terminal's and VS Code's
// settings.json), keeping every other byte as it was: the user's comments,
// order and formatting stay.
package jsonc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// member is one key of an object: offsets into the file.
type member struct {
	key              string
	keyStart         int
	valStart, valEnd int
}

// clean is src with comments and trailing commas turned into spaces (line
// breaks kept), so that offsets into it are offsets into src.
func clean(src []byte) []byte {
	b := append([]byte(nil), src...)
	if bytes.HasPrefix(b, []byte("\ufeff")) {
		copy(b, "   ")
	}
	for i := 0; i < len(b); i++ {
		switch {
		case b[i] == '"':
			i = skipString(b, i) - 1
		case b[i] == '/' && i+1 < len(b) && b[i+1] == '/':
			for ; i < len(b) && b[i] != '\n'; i++ {
				b[i] = ' '
			}
		case b[i] == '/' && i+1 < len(b) && b[i+1] == '*':
			end := len(b)
			if j := bytes.Index(b[i+2:], []byte("*/")); j >= 0 {
				end = i + 2 + j + 2
			}
			for ; i < end; i++ {
				if b[i] != '\n' {
					b[i] = ' '
				}
			}
			i--
		}
	}
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case '"':
			i = skipString(b, i) - 1
		case ',':
			if j := ws(b, i+1); j < len(b) && (b[j] == '}' || b[j] == ']') {
				b[i] = ' '
			}
		}
	}
	return b
}

// skipString is the offset after the string starting at b[i] (a quote).
func skipString(b []byte, i int) int {
	for j := i + 1; j < len(b); j++ {
		switch b[j] {
		case '\\':
			j++
		case '"':
			return j + 1
		}
	}
	return len(b)
}

// ws skips white space from i.
func ws(b []byte, i int) int {
	for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\r' || b[i] == '\n') {
		i++
	}
	return i
}

var errSyntax = errors.New("not valid JSON")

// value is the offset after the value starting at b[i].
func value(b []byte, i int) (int, error) {
	if i >= len(b) {
		return 0, errSyntax
	}
	switch b[i] {
	case '{':
		_, end, err := object(b, i)
		return end, err
	case '[':
		i = ws(b, i+1)
		if i < len(b) && b[i] == ']' {
			return i + 1, nil
		}
		for {
			end, err := value(b, ws(b, i))
			if err != nil {
				return 0, err
			}
			i = ws(b, end)
			switch {
			case i < len(b) && b[i] == ',':
				i++
			case i < len(b) && b[i] == ']':
				return i + 1, nil
			default:
				return 0, errSyntax
			}
		}
	case '"':
		end := skipString(b, i)
		if b[end-1] != '"' || end == i+1 {
			return 0, errSyntax
		}
		return end, nil
	}
	j := i
	for j < len(b) && !strings.ContainsRune(",}] \t\r\n", rune(b[j])) {
		j++
	}
	if j == i {
		return 0, errSyntax
	}
	return j, nil
}

// object reads the object starting at b[i] ('{'): its members and the
// offset after it.
func object(b []byte, i int) ([]member, int, error) {
	if i >= len(b) || b[i] != '{' {
		return nil, 0, errSyntax
	}
	var ms []member
	i = ws(b, i+1)
	if i < len(b) && b[i] == '}' {
		return nil, i + 1, nil
	}
	for {
		if i >= len(b) || b[i] != '"' {
			return nil, 0, errSyntax
		}
		keyEnd := skipString(b, i)
		var key string
		if err := json.Unmarshal(b[i:keyEnd], &key); err != nil {
			return nil, 0, errSyntax
		}
		c := ws(b, keyEnd)
		if c >= len(b) || b[c] != ':' {
			return nil, 0, errSyntax
		}
		vs := ws(b, c+1)
		ve, err := value(b, vs)
		if err != nil {
			return nil, 0, err
		}
		ms = append(ms, member{key: key, keyStart: i, valStart: vs, valEnd: ve})
		i = ws(b, ve)
		switch {
		case i < len(b) && b[i] == ',':
			i = ws(b, i+1)
		case i < len(b) && b[i] == '}':
			return ms, i + 1, nil
		default:
			return nil, 0, errSyntax
		}
	}
}

// find is the last member named key (the one JSON readers keep).
func find(ms []member, key string) *member {
	for i := len(ms) - 1; i >= 0; i-- {
		if ms[i].key == key {
			return &ms[i]
		}
	}
	return nil
}

// Get returns the string at path (object keys). ok is false when a key is
// missing or the value is not a string.
func Get(src []byte, path ...string) (s string, ok bool) {
	b := clean(src)
	i := ws(b, 0)
	for d, key := range path {
		ms, _, err := object(b, i)
		if err != nil {
			return "", false
		}
		m := find(ms, key)
		if m == nil {
			return "", false
		}
		if d == len(path)-1 {
			if json.Unmarshal(b[m.valStart:m.valEnd], &s) != nil {
				return "", false
			}
			return s, true
		}
		i = m.valStart
	}
	return "", false
}

// Set puts the string value at path: a value there is replaced; missing
// keys (and the objects they need) are added first in their object,
// indented like the file. Nothing else in src changes.
func Set(src []byte, value string, path ...string) ([]byte, error) {
	if len(path) == 0 {
		return nil, errors.New("jsonc: empty path")
	}
	b := clean(src)
	i := ws(b, 0)
	for d, key := range path {
		ms, end, err := object(b, i)
		if err != nil {
			if d == 0 {
				return nil, err
			}
			return nil, fmt.Errorf("%s is not an object", strings.Join(path[:d], "."))
		}
		m := find(ms, key)
		if m == nil {
			return insert(src, i, end, ms, quote(key)+": "+nested(path[d+1:], value)), nil
		}
		if d == len(path)-1 {
			out := append([]byte(nil), src[:m.valStart]...)
			out = append(out, quote(value)...)
			return append(out, src[m.valEnd:]...), nil
		}
		i = m.valStart
	}
	return nil, errSyntax
}

func quote(s string) string {
	q, _ := json.Marshal(s)
	return string(q)
}

// nested is the value for the rest of a path: {"a": {"b": "value"}}.
func nested(rest []string, value string) string {
	if len(rest) == 0 {
		return quote(value)
	}
	return "{" + quote(rest[0]) + ": " + nested(rest[1:], value) + "}"
}

// insert adds member text first in the object src[start:end].
func insert(src []byte, start, end int, ms []member, text string) []byte {
	nl := "\n"
	if bytes.Contains(src, []byte("\r\n")) {
		nl = "\r\n"
	}
	outer := indentAt(src, start)
	inner := outer + unit(src)
	if len(ms) > 0 && lineStart(src, ms[0].keyStart) {
		inner = indentAt(src, ms[0].keyStart)
	}
	var mid string
	switch {
	case len(ms) > 0:
		mid = nl + inner + text + ","
	case len(bytes.TrimSpace(src[start+1:end-1])) == 0:
		// An empty object (no comment in it) is written whole.
		out := append([]byte(nil), src[:start]...)
		out = append(out, "{"+nl+inner+text+nl+outer+"}"...)
		return append(out, src[end:]...)
	default:
		mid = nl + inner + text
	}
	out := append([]byte(nil), src[:start+1]...)
	out = append(out, mid...)
	return append(out, src[start+1:]...)
}

// indentAt is the white space starting the line of offset i.
func indentAt(src []byte, i int) string {
	ls := bytes.LastIndexByte(src[:i], '\n') + 1
	j := ls
	for j < len(src) && (src[j] == ' ' || src[j] == '\t') {
		j++
	}
	return string(src[ls:j])
}

// lineStart reports whether only white space comes before offset i on its line.
func lineStart(src []byte, i int) bool {
	ls := bytes.LastIndexByte(src[:i], '\n') + 1
	return len(bytes.TrimSpace(src[ls:i])) == 0
}

// unit is the file's indentation step: its first indented line's (four
// spaces when there is none).
func unit(src []byte) string {
	for _, l := range strings.Split(string(src), "\n") {
		t := strings.TrimLeft(l, " \t")
		if t != "" && len(t) < len(l) {
			return l[:len(l)-len(t)]
		}
	}
	return "    "
}
