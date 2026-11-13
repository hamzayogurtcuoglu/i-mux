package sidebar

import (
	"os"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/gitops"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// typeIn feeds keys to b: single characters as typed, longer words as the
// named keys.
func typeIn(b *CommitBox, keys ...string) (last int) {
	for _, k := range keys {
		last, _ = b.Key(Event{Kind: EvKey, Key: k})
	}
	return last
}

// boxText is b's text with "|" where the cursor is.
func boxText(b CommitBox) string {
	return string(b.Text[:b.Cursor]) + "|" + string(b.Text[b.Cursor:])
}

// TestCommitBoxKeys (T70.1): typing, moving and deleting as an editor's
// field does; Ctrl+J and Alt+Enter a new line, a paste as it is (CR and
// CRLF one newline, a tab a space, control characters out); Enter commits,
// Esc and Tab leave; typing ends the hint that asks for a message.
func TestCommitBoxKeys(t *testing.T) {
	b := &CommitBox{Warn: true}
	for _, k := range strings.Split("fix: the bar", "") {
		typeIn(b, k)
	}
	if boxText(*b) != "fix: the bar|" || b.Warn {
		t.Fatalf("typed %q warn %v", boxText(*b), b.Warn)
	}
	for _, c := range []struct {
		keys []string
		want string
	}{
		{[]string{"left", "left", "left"}, "fix: the |bar"},
		{[]string{"backspace"}, "fix: the|bar"},
		{[]string{"delete"}, "fix: the|ar"},
		{[]string{" ", "c"}, "fix: the c|ar"},
		{[]string{"home"}, "|fix: the car"},
		{[]string{"right", "right", "end"}, "fix: the car|"},
		{[]string{"ctrl-w"}, "fix: the |"},
		{[]string{"ctrl-a", "ctrl-e", "ctrl-u"}, "|"},
		{[]string{"left", "backspace", "delete"}, "|"},
	} {
		if typeIn(b, c.keys...); boxText(*b) != c.want {
			t.Errorf("%v: %q, want %q", c.keys, boxText(*b), c.want)
		}
	}
	// Lines: a subject, a blank line, a body; ↑ ↓ keep the column where
	// the line has it.
	typeIn(b, "a", "b", "c")
	b.Key(Event{Kind: EvKey, Key: "enter", Text: "\n"}) // Ctrl+J
	typeIn(b, "alt-enter", "d", "e", "f", "g", "up")
	if boxText(*b) != "abc\n|\ndefg" {
		t.Errorf("Ctrl+J, Alt+Enter, up: %q", boxText(*b))
	}
	if typeIn(b, "down", "end", "down"); b.Cursor != len(b.Text) {
		t.Errorf("down on the last line: %q", boxText(*b))
	}
	two := &CommitBox{Text: []rune("abc\ndefg"), Cursor: 6}
	if typeIn(two, "up"); boxText(*two) != "ab|c\ndefg" {
		t.Errorf("up: %q", boxText(*two))
	}
	if typeIn(two, "down", "end", "up"); boxText(*two) != "abc|\ndefg" {
		t.Errorf("down, end, up: %q", boxText(*two))
	}
	if typeIn(two, "up"); two.Cursor != 0 {
		t.Errorf("up on the first line: %q", boxText(*two))
	}
	// A paste goes in as it is, CR and CRLF one newline.
	p := &CommitBox{}
	if _, changed := p.Key(Event{Kind: EvPaste, Text: "one\r\ntwo\rthree\tfour\x07\x1b ü"}); !changed || boxText(*p) != "one\ntwo\nthree four ü|" {
		t.Errorf("paste %q", boxText(*p))
	}
	// Enter commits; Esc, Tab and Ctrl+C leave; other keys do nothing.
	for k, want := range map[string]int{"enter": boxSubmit, "esc": boxLeave, "tab": boxLeave, "backtab": boxLeave, "ctrl-c": boxLeave, "pgup": boxTyped} {
		if got := typeIn(p, k); got != want {
			t.Errorf("%s: %d, want %d", k, got, want)
		}
	}
	if boxText(*p) != "one\ntwo\nthree four ü|" {
		t.Errorf("a key that is not typed changed it: %q", boxText(*p))
	}
	// Spaces are no message; a long paste is cut.
	if !(CommitBox{Text: []rune(" \n ")}).Empty() || (CommitBox{Text: []rune("x")}).Empty() {
		t.Error("Empty")
	}
	big := &CommitBox{}
	big.Key(Event{Kind: EvPaste, Text: strings.Repeat("x", maxBoxRunes+10)})
	if len(big.Text) != maxBoxRunes {
		t.Errorf("a long paste: %d runes", len(big.Text))
	}
}

// TestWrapBox (T70.1): rows break at a newline and at the last space of a
// long line (the space goes), else where a row is full; a text ending with
// a newline has an empty row last, for the cursor.
func TestWrapBox(t *testing.T) {
	rows := func(s string, w int) []string {
		rs := []rune(s)
		var out []string
		for _, sg := range wrapBox(rs, w) {
			out = append(out, string(rs[sg.from:sg.to]))
		}
		return out
	}
	for _, c := range []struct {
		in   string
		w    int
		want string
	}{
		{"", 10, ""},
		{"fix the bar", 20, "fix the bar"},
		{"fix the sidebar width", 10, "fix the|sidebar|width"},
		{"hello world", 5, "hello|world"},
		{"abcdefghij", 4, "abcd|efgh|ij"},
		{"subject\n\nbody", 20, "subject||body"},
		{"end\n", 20, "end|"},
		{"ğüşçö🙂🙂", 4, "ğüşç|ö🙂|🙂"},
	} {
		if got := strings.Join(rows(c.in, c.w), "|"); got != c.want {
			t.Errorf("wrapBox(%q, %d) = %q, want %q", c.in, c.w, got, c.want)
		}
	}
	segs := wrapBox([]rune("hello world"), 5)
	if boxRow(segs, 5) != 0 || boxRow(segs, 6) != 1 || boxRow(segs, 11) != 1 {
		t.Errorf("cursor rows %v", segs)
	}
}

// boxLines are the rendered rows of the box in sc: their text and actions.
func boxLines(sc Screen) []Line {
	var out []Line
	for _, l := range sc.Lines {
		for _, c := range l.Cells {
			if c.Action.Kind == ActCommitBox {
				out = append(out, Line{Text: c.Text, Action: c.Action})
			}
		}
	}
	return out
}

// TestCommitBoxView (T70.1): the box sits between the branch row and
// Commit with VS Code's hint, wraps its text and grows to six rows (fewer
// in a short sidebar), the cursor in reverse video while it has the keys
// (not while the pane is away), the hint in yellow when a message is asked
// for; every row the sidebar's width in both icon themes; a click on a row
// puts the cursor where it was clicked.
func TestCommitBoxView(t *testing.T) {
	i18n.SetLocale("en")
	m := scmModel()
	for _, theme := range []string{IconsEmoji, IconsNerd} {
		m.Icons = theme
		for _, text := range []string{"", "fix: keep the sidebar's width when a client attaches", strings.Repeat("word ", 80), "a\n\n" + strings.Repeat("x", 300)} {
			m.Commit = CommitBox{Text: []rune(text), Cursor: len([]rune(text)), Focused: text != ""}
			for w := config.MinSidebarWidth; w <= config.MaxSidebarWidth; w++ {
				sc := Render(m, w, 40, NoItem)
				for _, l := range sc.Lines {
					if !strings.Contains(Plain(l.Text), "Commit") && l.ActionAt(w-2).Kind != ActCommitBox && w < 43 {
						continue // the narrow sidebar's other rows are not the box's
					}
					if n := DisplayWidth(Plain(l.Text)); n != w {
						t.Fatalf("%s %d %.20q: %d columns: %q", theme, w, text, n, Plain(l.Text))
					}
				}
				if n := len(boxLines(sc)); n < 1 || n > maxBoxRows {
					t.Fatalf("%s %d: %d rows", theme, w, n)
				}
			}
		}
	}
	m.Icons = IconsEmoji

	// Empty: the hint, under the branch, over Commit.
	m.Commit = CommitBox{}
	sc := Render(m, 50, 40, NoItem)
	text := screenText(sc)
	branch, box, commit := strings.Index(text, "🌿 main"), strings.Index(text, "Message (Enter to commit)"), strings.Index(text, "✅ Commit")
	if branch < 0 || box < branch || commit < box || len(boxLines(sc)) != 1 {
		t.Fatalf("the box's place:\n%s", text)
	}
	if strings.Contains(boxLines(sc)[0].Text, rev) || !strings.Contains(boxLines(sc)[0].Text, boxBg) {
		t.Errorf("unfocused %q", boxLines(sc)[0].Text)
	}
	// Asked for: the hint in yellow, naming the commit; the cursor first.
	m.Commit = CommitBox{Focused: true, Warn: true}
	l := boxLines(Render(m, 50, 40, NoItem))[0]
	if p := Plain(l.Text); !strings.HasPrefix(p, "▎  Type a message, then Enter") || !strings.Contains(l.Text, yellow) || !strings.Contains(l.Text, rev+" "+revOff) {
		t.Errorf("warn %q", l.Text)
	}
	m.Commit.Ask = "commit-push"
	if p := Plain(boxLines(Render(m, 43, 40, NoItem))[0].Text); !strings.Contains(p, "Message (Commit & Push) ") {
		t.Errorf("ask %q", p)
	}

	// Text: wrapped at a space (34 columns: 29 of text, one for the
	// cursor at a row's end), the cursor on its character; away: none.
	m.Commit = CommitBox{Text: []rune("fix: keep the sidebar's width when a client attaches"), Cursor: 5, Focused: true}
	ls := boxLines(Render(m, 43, 40, NoItem))
	var rows []string
	for _, l := range ls {
		rows = append(rows, strings.TrimSpace(Plain(l.Text)))
	}
	if strings.Join(rows, "|") != "▎ fix: keep the sidebar's width|▎ when a client attaches" {
		t.Errorf("rows %q", rows)
	}
	if !strings.Contains(ls[0].Text, rev+"k"+revOff) || !strings.Contains(ls[0].Text, cyan+"▎") || !strings.Contains(ls[0].Text, boxFocusBg) {
		t.Errorf("cursor %q", ls[0].Text)
	}
	if ls[0].Action != (Action{Kind: ActCommitBox, Section: "0:29"}) || ls[1].Action.Section != "30:52" {
		t.Errorf("actions %+v %+v", ls[0].Action, ls[1].Action)
	}
	m.Commit.Away = true
	if strings.Contains(boxLines(Render(m, 43, 40, NoItem))[0].Text, rev) {
		t.Error("a cursor while the pane is away")
	}

	// Taller than six rows: the rows around the cursor, … for the others;
	// a short sidebar: fewer rows.
	long := strings.TrimSpace(strings.Repeat("word ", 80))
	m.Commit = CommitBox{Text: []rune(long), Cursor: 0, Focused: true}
	ls = boxLines(Render(m, 43, 40, NoItem))
	if len(ls) != maxBoxRows || !strings.HasSuffix(strings.TrimRight(Plain(ls[5].Text), " "), "…") || !strings.Contains(ls[0].Text, rev+"w") {
		t.Errorf("long, cursor at the top: %d rows %q", len(ls), Plain(ls[len(ls)-1].Text))
	}
	m.Commit.Cursor = len(m.Commit.Text)
	ls = boxLines(Render(m, 43, 40, NoItem))
	if !strings.Contains(ls[5].Text, rev+" "+revOff) || !strings.HasSuffix(strings.TrimRight(Plain(ls[0].Text), " "), "…") {
		t.Errorf("long, cursor at the end: %q / %q", Plain(ls[0].Text), Plain(ls[5].Text))
	}
	if n := len(boxLines(Render(m, 43, 14, NoItem))); n != 2 {
		t.Errorf("a short sidebar: %d rows", n)
	}

	// A click: on the character clicked, past the text at the row's end.
	b := &CommitBox{Text: []rune("fix: keep the sidebar's width when a client attaches")}
	for col, want := range map[int]int{0: 0, 4: 4, 36: 36, 37: 37, 60: 37, -3: 0} {
		b.place(0, 37, col)
		if b.Cursor != want {
			t.Errorf("click at %d: cursor %d, want %d", col, b.Cursor, want)
		}
	}
	b.place(38, 52, 3)
	if b.Cursor != 41 {
		t.Errorf("click on the second row: %d", b.Cursor)
	}
}

// TestCommitDraft (T70.1): a project's message is kept in a 0600 file
// while it is written, apart from another project's; empty removes it; a
// commit removes it unless it was changed meanwhile.
func TestCommitDraft(t *testing.T) {
	testutil.Isolate(t)
	a, b := t.TempDir(), t.TempDir()
	if CommitDraft(a) != "" {
		t.Fatal("a message before any")
	}
	if err := SetCommitDraft(a, "fix: one\n\nbody ü"); err != nil {
		t.Fatal(err)
	}
	if err := SetCommitDraft(b, "feat: two"); err != nil {
		t.Fatal(err)
	}
	if CommitDraft(a) != "fix: one\n\nbody ü" || CommitDraft(b) != "feat: two" || CommitDraft(a+"/") != CommitDraft(a) {
		t.Fatalf("read back %q %q", CommitDraft(a), CommitDraft(b))
	}
	if fi, err := os.Stat(draftPath(a)); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("file %v %v", fi, err)
	}
	ClearCommitDraft(b, "feat: changed")
	if CommitDraft(b) != "feat: two" {
		t.Error("a message changed meanwhile was removed")
	}
	ClearCommitDraft(b, "feat: two")
	if CommitDraft(b) != "" {
		t.Error("the commit's message stayed")
	}
	if err := SetCommitDraft(a, ""); err != nil || CommitDraft(a) != "" {
		t.Fatalf("emptied: %v %q", err, CommitDraft(a))
	}
	if _, err := os.Stat(draftPath(a)); !os.IsNotExist(err) {
		t.Errorf("the file stayed: %v", err)
	}
	if err := SetCommitDraft(a, ""); err != nil {
		t.Errorf("emptied twice: %v", err)
	}
}

// TestCommitButton (T70.2): Commit commits the box's message; during a
// merge, git's message unless one is typed; Continue during a rebase.
func TestCommitButton(t *testing.T) {
	i18n.SetLocale("en")
	m := scmModel()
	if label, a := commitButton(m); label != "✅ Commit" || a.Kind != ActCommit {
		t.Errorf("%q %+v", label, a)
	}
	m.SCM.Op = gitops.OpMerge
	if label, a := commitButton(m); label != "✅ Commit Merge" || a != (Action{Kind: ActGitPopup, Section: "continue"}) {
		t.Errorf("merge %q %+v", label, a)
	}
	m.Commit.Text = []rune("Merge the fix")
	if _, a := commitButton(m); a.Kind != ActCommit {
		t.Errorf("merge with a message %+v", a)
	}
	m.SCM.Op = gitops.OpRebase
	if label, a := commitButton(m); label != "✅ Continue" || a.Kind != ActGitPopup {
		t.Errorf("rebase %q %+v", label, a)
	}
}
