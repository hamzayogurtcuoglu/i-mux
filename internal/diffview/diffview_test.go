package diffview

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

var esc = regexp.MustCompile(`\x1b\[[0-9;]*m`)

var unified = `diff --git a/a.go b/a.go
index 1..2 100644
--- a/a.go
+++ b/a.go
@@ -1,6 +1,7 @@
 package a
 
-func old() {}
+func renamed() {}
+func added() {}
 
 // tail
` + " \ttabbed\n" + `\ No newline at end of file
`

func TestParse(t *testing.T) {
	ls := Parse(unified)
	if len(ls) != 8 {
		t.Fatalf("%d lines: %+v", len(ls), ls)
	}
	if ls[2] != (Line{Kind: Removed, OldNo: 3, Text: "func old() {}"}) || ls[3] != (Line{Kind: Added, NewNo: 3, Text: "func renamed() {}"}) ||
		ls[4] != (Line{Kind: Added, NewNo: 4, Text: "func added() {}"}) || ls[7].OldNo != 6 || ls[7].NewNo != 7 || ls[7].Text != "\ttabbed" {
		t.Fatalf("lines %+v", ls)
	}
	if a, r := Stats(ls); a != 2 || r != 1 {
		t.Fatalf("stats %d %d", a, r)
	}
	if n := AllAdded("x\ny\n"); len(n) != 2 || n[1] != (Line{Kind: Added, NewNo: 2, Text: "y"}) {
		t.Fatalf("all added %+v", n)
	}
}

func TestRender(t *testing.T) {
	ls := Parse(unified)
	for _, side := range []bool{true, false} {
		out, first := Render(ls, 80, side)
		if first != 2 {
			t.Errorf("side=%v first change %d", side, first)
		}
		for _, l := range out {
			if n := utf8.RuneCountInString(esc.ReplaceAllString(l, "")); n > 80 {
				t.Errorf("side=%v %d columns: %q", side, n, l)
			}
		}
		text := esc.ReplaceAllString(strings.Join(out, "\n"), "")
		t.Logf("\n%s", text)
		for _, want := range []string{"func old() {}", "func renamed() {}", "func added() {}", "    tabbed"} {
			if !strings.Contains(text, want) {
				t.Errorf("side=%v lacks %q", side, want)
			}
		}
	}
	// Side by side: the removed line and the first added one share a row.
	out, _ := Render(ls, 80, true)
	row := esc.ReplaceAllString(out[2], "")
	if !strings.Contains(row, "3 - func old() {}") || !strings.Contains(row, "3 + func renamed() {}") {
		t.Errorf("paired row %q", row)
	}
	if !strings.Contains(out[2], redBg) || !strings.Contains(out[2], greenBg) || !strings.Contains(out[3], emptyBg) {
		t.Error("colors")
	}
}
