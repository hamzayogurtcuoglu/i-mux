package sidebar

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
)

// The Git view's commit message box, like VS Code's (§9.3 "The commit
// message box", D65): the next commit's message is typed in it, above the
// Commit button. A project's message is kept in a file while it is
// written: every sidebar of the project shows it, a restart keeps it, and
// `imux __git commit --draft` reads it from there, never from a command
// line.

// OptCommitAsk is the option of a sidebar's pane that a commit item of a
// menu sets when the box is empty: the box asks for the message of that
// commit (a CommitAsks key), which Enter then makes.
const OptCommitAsk = "@imux_commit_ask"

// CommitAsks are the commits the box may be asked for, with the i18n keys
// of their menu items. An amend with no message keeps the last one: it
// never asks.
var CommitAsks = map[string]string{
	"commit":                "scm.commit_default",
	"commit-staged":         "scm.commit_staged",
	"commit-all":            "scm.commit_all",
	"commit-signoff":        "scm.commit_signoff",
	"commit-staged-signoff": "scm.commit_staged_signoff",
	"commit-all-signoff":    "scm.commit_all_signoff",
	"commit-push":           "scm.commit_push",
	"commit-sync":           "scm.commit_sync",
}

// CommitBox is the box's state, a sidebar's; Model.Commit draws it.
type CommitBox struct {
	Text    []rune
	Cursor  int    // where typing goes, 0 … len(Text)
	Focused bool   // the cursor is in it: the keys are typed into it
	Away    bool   // the pane lost the focus: no cursor until it is back
	Ask     string // the commit Enter makes (a CommitAsks key); "": the Commit button's
	Warn    bool   // a commit was asked for with no message: the hint says to type one
}

const (
	// maxBoxRows is how tall the box grows (fewer rows in a short sidebar).
	maxBoxRows = 6
	// maxBoxRunes bounds a message (a long paste).
	maxBoxRunes = 20000
	// The box's backgrounds: VS Code's input field, a step lighter while
	// the cursor is in it.
	boxBg      = "\x1b[48;5;236m"
	boxFocusBg = "\x1b[48;5;237m"
	revOff     = "\x1b[27m"
	fgDefault  = "\x1b[39m"
)

// What a key did in the box.
const (
	boxTyped  = iota // typed, moved, or nothing
	boxSubmit        // Enter: what the Commit button does
	boxLeave         // Esc, Tab: the cursor leaves the box
)

// Message is the box's text.
func (b CommitBox) Message() string { return string(b.Text) }

// Empty reports whether no message is typed (spaces are none).
func (b CommitBox) Empty() bool { return strings.TrimSpace(string(b.Text)) == "" }

func (b *CommitBox) clamp() { b.Cursor = min(max(b.Cursor, 0), len(b.Text)) }

// insert types s at the cursor: its newlines kept (CR and CRLF become
// one), a tab a space, other control characters left out.
func (b *CommitBox) insert(s string) bool {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	var add []rune
	for _, r := range s {
		switch {
		case r == '\n':
		case r == '\t':
			r = ' '
		case r < ' ' || r == 0x7f || r >= 0x80 && r < 0xa0 || r == utf8.RuneError:
			continue
		}
		add = append(add, r)
	}
	if room := maxBoxRunes - len(b.Text); len(add) > room {
		add = add[:max(room, 0)]
	}
	if len(add) == 0 {
		return false
	}
	b.clamp()
	t := make([]rune, 0, len(b.Text)+len(add))
	t = append(append(append(t, b.Text[:b.Cursor]...), add...), b.Text[b.Cursor:]...)
	b.Text, b.Cursor = t, b.Cursor+len(add)
	return true
}

// cut deletes Text[from:to]; the cursor goes to from.
func (b *CommitBox) cut(from, to int) bool {
	from, to = max(from, 0), min(to, len(b.Text))
	if from >= to {
		return false
	}
	b.Text = append(append([]rune{}, b.Text[:from]...), b.Text[to:]...)
	b.Cursor = from
	return true
}

// lineStart and lineEnd are the ends of the line i is on.
func (b *CommitBox) lineStart(i int) int {
	for i > 0 && b.Text[i-1] != '\n' {
		i--
	}
	return i
}

func (b *CommitBox) lineEnd(i int) int {
	for i < len(b.Text) && b.Text[i] != '\n' {
		i++
	}
	return i
}

// Key applies a key or a paste; it reports what it did, and whether the
// text changed.
func (b *CommitBox) Key(ev Event) (int, bool) {
	b.clamp()
	if ev.Kind == EvPaste {
		return boxTyped, b.edited(b.insert(ev.Text))
	}
	if ev.Kind != EvKey {
		return boxTyped, false
	}
	c := b.Cursor
	switch k := ev.Key; k {
	case "enter":
		if ev.Text == "\n" { // Ctrl+J
			return boxTyped, b.edited(b.insert("\n"))
		}
		return boxSubmit, false
	case "alt-enter":
		return boxTyped, b.edited(b.insert("\n"))
	case "esc", "tab", "backtab", "ctrl-c":
		return boxLeave, false
	case "backspace":
		return boxTyped, b.edited(b.cut(c-1, c))
	case "delete":
		return boxTyped, b.edited(b.cut(c, c+1))
	case "ctrl-u":
		return boxTyped, b.edited(b.cut(b.lineStart(c), c))
	case "ctrl-w":
		i := c
		for i > 0 && b.Text[i-1] == ' ' {
			i--
		}
		for i > 0 && b.Text[i-1] != ' ' && b.Text[i-1] != '\n' {
			i--
		}
		return boxTyped, b.edited(b.cut(i, c))
	case "left":
		b.Cursor--
	case "right":
		b.Cursor++
	case "home", "ctrl-a":
		b.Cursor = b.lineStart(c)
	case "end", "ctrl-e":
		b.Cursor = b.lineEnd(c)
	case "up":
		if ls := b.lineStart(c); ls == 0 {
			b.Cursor = 0
		} else {
			b.Cursor = min(b.lineStart(ls-1)+c-ls, ls-1)
		}
	case "down":
		if le := b.lineEnd(c); le == len(b.Text) {
			b.Cursor = le
		} else {
			b.Cursor = min(le+1+c-b.lineStart(c), b.lineEnd(le+1))
		}
	default:
		if r, size := utf8.DecodeRuneInString(k); size == len(k) && r >= ' ' && r != utf8.RuneError {
			return boxTyped, b.edited(b.insert(k))
		}
	}
	b.clamp()
	return boxTyped, false
}

// edited notes a change: the hint asking for a message goes.
func (b *CommitBox) edited(changed bool) bool {
	if changed {
		b.Warn = false
	}
	return changed
}

// place puts the cursor at column col of the row Text[from:to] (a click
// there): on the character clicked, at the row's end past its text.
func (b *CommitBox) place(from, to, col int) {
	from, to = min(max(from, 0), len(b.Text)), min(max(to, 0), len(b.Text))
	p, used := from, 0
	for p < to {
		cw := runeWidth(b.Text[p])
		if used+cw > col {
			break
		}
		used += cw
		p++
	}
	b.Cursor = p
}

// boxSeg is one row of the box: Text[from:to].
type boxSeg struct{ from, to int }

// wrapBox lays the text out in rows of at most w columns: a new row at
// each \n; a long line breaks at its last space (the space goes), else
// where the row is full. A text ending with \n has an empty row last, for
// the cursor.
func wrapBox(rs []rune, w int) []boxSeg {
	w = max(w, 1)
	var out []boxSeg
	start := 0
	for {
		col, i, space := 0, start, -1
		for i < len(rs) && rs[i] != '\n' {
			cw := runeWidth(rs[i])
			if col+cw > w {
				break
			}
			if rs[i] == ' ' {
				space = i
			}
			col += cw
			i++
		}
		switch {
		case i >= len(rs):
			return append(out, boxSeg{start, len(rs)})
		case rs[i] == '\n', rs[i] == ' ':
			out = append(out, boxSeg{start, i})
			start = i + 1
		case space > start:
			out = append(out, boxSeg{start, space})
			start = space + 1
		default:
			if i == start {
				i++ // one character wider than the row
			}
			out = append(out, boxSeg{start, i})
			start = i
		}
	}
}

// boxRow is the row the cursor at c is on.
func boxRow(segs []boxSeg, c int) int {
	r := 0
	for i, s := range segs {
		if s.from <= c {
			r = i
		}
	}
	return r
}

// commitBoxRows draws the box for w columns, in at most n rows: a margin
// (▎ while the cursor is in it), the field with its text wrapped, or VS
// Code's hint when it is empty. Each row's action places the cursor where
// it is clicked.
func (m Model) commitBoxRows(w, n int) []Line {
	b := m.Commit
	iw := max(w-4, 4) // the field's columns inside its padding
	show := b.Focused && !b.Away
	lead, bg := " ", boxBg
	if b.Focused {
		lead, bg = cyan+"▎"+reset, boxFocusBg
	}
	row := func(content string, from, to int) Line {
		return Line{
			Text:   lead + bg + " " + content + " " + reset + " ",
			Action: Action{Kind: ActCommitBox, Section: strconv.Itoa(from) + ":" + strconv.Itoa(to)},
		}
	}
	if len(b.Text) == 0 {
		hint, style := i18n.T("scm.box_hint"), muted
		switch {
		case CommitAsks[b.Ask] != "":
			hint, style = i18n.T("scm.box_ask", i18n.T(CommitAsks[b.Ask])), yellow
		case b.Warn:
			hint, style = i18n.T("scm.box_empty"), yellow
		}
		content, used := "", 0
		if show {
			content, used = rev+" "+revOff, 1
		}
		return []Line{row(content+style+fit(hint, iw-used)+fgDefault, 0, 0)}
	}
	segs := wrapBox(b.Text, iw-1) // a column left for the cursor at a row's end
	cur := boxRow(segs, b.Cursor)
	n = max(n, 1)
	first := 0
	if len(segs) > n && show {
		first = min(max(cur-n+1, 0), len(segs)-n)
	}
	last := min(first+n, len(segs))
	var out []Line
	for r := first; r < last; r++ {
		sg := segs[r]
		var sb strings.Builder
		used := 0
		for i := sg.from; i < sg.to; i++ {
			ch := string(b.Text[i])
			if show && i == b.Cursor {
				ch = rev + ch + revOff
			}
			sb.WriteString(ch)
			used += runeWidth(b.Text[i])
		}
		if show && r == cur && b.Cursor == sg.to {
			sb.WriteString(rev + " " + revOff)
			used++
		}
		pad := max(iw-used, 0)
		if more := r == last-1 && last < len(segs) || r == first && first > 0; more && pad > 0 {
			sb.WriteString(strings.Repeat(" ", pad-1) + faint + "…" + fgDefault) // more above or below
		} else {
			sb.WriteString(strings.Repeat(" ", pad))
		}
		out = append(out, row(sb.String(), sg.from, sg.to))
	}
	return out
}

// draftPath is the file that keeps dir's message while it is written.
func draftPath(dir string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(dir)))
	return filepath.Join(paths.Home(), "commit", hex.EncodeToString(sum[:8])+".txt")
}

// CommitDraft is the message typed in dir's commit box ("" for none).
func CommitDraft(dir string) string {
	if dir == "" {
		return ""
	}
	b, err := os.ReadFile(draftPath(dir))
	if err != nil {
		return ""
	}
	return strings.ToValidUTF8(string(b), "")
}

// SetCommitDraft keeps msg as dir's message ("" removes it). It is
// written as it is typed: a new file renamed over the old one, without a
// sync (a crash loses a few keys at most).
func SetCommitDraft(dir, msg string) error {
	if dir == "" {
		return nil
	}
	p := draftPath(dir)
	if msg == "" {
		if err := os.Remove(p); err != nil && !paths.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := paths.EnsureDir(filepath.Dir(p)); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".draft-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, werr := f.WriteString(msg)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return werr
	}
	return os.Rename(f.Name(), p)
}

// ClearCommitDraft removes dir's message once a commit used it, unless it
// was changed meanwhile.
func ClearCommitDraft(dir, msg string) {
	if CommitDraft(dir) == msg {
		_ = SetCommitDraft(dir, "")
	}
}
