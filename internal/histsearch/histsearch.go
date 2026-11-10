// Package histsearch finds text in conversations: History's search (§9.3
// "Search in History"). A conversation is read whole only when its file can
// hold the text.
package histsearch

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// Snip is a message with the text: one line around it.
type Snip struct {
	User     bool // the user's message (else the agent's)
	Text     string
	Col, Len int // the match in Text, in runes
}

// Hit is a conversation with the text.
type Hit struct {
	Ref   session.Ref
	Count int    // its messages with the text
	Snips []Snip // the first MaxSnips of them
}

// MaxSnips is how many messages a hit shows.
const MaxSnips = 3

// Loader reads a whole conversation.
type Loader func(ctx context.Context, ref session.Ref) (*session.Session, error)

// Whole are load options that read a conversation from start to end.
var Whole = session.LoadOptions{HeadBytes: 1 << 50, TailBytes: 1 << 50}

// Search sends the conversations among refs that have q, in refs' order,
// then closes out. Uppercase in q makes it case-sensitive (as the project
// search). done, when not nil, gets each conversation looked at.
func Search(ctx context.Context, refs []session.Ref, q string, load Loader, out chan<- Hit, done func()) {
	defer close(out)
	if strings.TrimSpace(q) == "" {
		return
	}
	fold := !hasUpper(q)
	for _, r := range refs {
		if ctx.Err() != nil {
			return
		}
		if maybe(r, q, fold) {
			if s, err := load(ctx, r); err == nil && s != nil {
				if h, ok := Find(s, q); ok {
					h.Ref = r
					select {
					case out <- h:
					case <-ctx.Done():
						return
					}
				}
			}
		}
		if done != nil {
			done()
		}
	}
}

// Find looks for q in the messages of s.
func Find(s *session.Session, q string) (Hit, bool) {
	fold := !hasUpper(q)
	needle := []rune(q)
	if fold {
		needle = lower(needle)
	}
	var h Hit
	for _, e := range s.Entries {
		if e.Role == session.RoleNotice || e.Kind == session.KindHandoff || e.Text == "" {
			continue
		}
		text := []rune(strings.Join(strings.Fields(e.Text), " "))
		hay := text
		if fold {
			hay = lower(text)
		}
		i := index(hay, needle)
		if i < 0 {
			continue
		}
		h.Count++
		if len(h.Snips) < MaxSnips {
			h.Snips = append(h.Snips, snip(text, i, len(needle), e.Role == session.RoleUser))
		}
	}
	return h, h.Count > 0
}

// snip is the part of text around the match at rune i (n runes): a little
// before it, more after.
func snip(text []rune, i, n int, user bool) Snip {
	const before, after = 24, 120
	start, end := max(i-before, 0), min(i+n+after, len(text))
	out := string(text[start:end])
	col := i - start
	if start > 0 {
		out = "…" + out
		col++
	}
	if end < len(text) {
		out += "…"
	}
	return Snip{User: user, Text: out, Col: col, Len: n}
}

// maybe reports whether r's file can hold q: false only when it was read
// and q is not in it. JSON escapes quotes, backslashes, and some writers <,
// > and & or every non-ASCII letter: such a q is looked for after parsing.
func maybe(r session.Ref, q string, fold bool) bool {
	for _, c := range q {
		if c > unicode.MaxASCII || c < ' ' || strings.ContainsRune(`"\<>&`, c) {
			return true
		}
	}
	path := r.Path
	if fi, err := os.Stat(path); err != nil {
		return true
	} else if fi.IsDir() {
		path = filepath.Join(path, "events.jsonl") // Copilot
	}
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()
	needle := []byte(q)
	if fold {
		needle = bytes.ToLower(needle)
	}
	rd := bufio.NewReaderSize(f, 1<<20)
	buf := make([]byte, 1<<20)
	var tail []byte
	for {
		n, err := io.ReadFull(rd, buf)
		if n > 0 {
			chunk := append(tail, buf[:n]...)
			if fold {
				chunk = bytes.ToLower(chunk)
			}
			if bytes.Contains(chunk, needle) {
				return true
			}
			if k := len(needle) - 1; k > 0 && len(chunk) >= k {
				tail = append([]byte(nil), chunk[len(chunk)-k:]...)
			}
		}
		if err != nil {
			return false
		}
	}
}

func hasUpper(s string) bool {
	for _, r := range s {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}

func lower(r []rune) []rune {
	out := make([]rune, len(r))
	for i, c := range r {
		out[i] = unicode.ToLower(c)
	}
	return out
}

// index is the first rune index of needle in hay, or -1.
func index(hay, needle []rune) int {
	if len(needle) == 0 {
		return -1
	}
outer:
	for i := 0; i+len(needle) <= len(hay); i++ {
		for j, c := range needle {
			if hay[i+j] != c {
				continue outer
			}
		}
		return i
	}
	return -1
}
