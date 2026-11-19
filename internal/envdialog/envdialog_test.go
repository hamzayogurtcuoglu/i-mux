package envdialog

import (
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func typeText(m *Model, s string) {
	for _, r := range s {
		m.Key(string(r))
	}
}

func screen(m *Model, w, h int) string {
	var b strings.Builder
	for _, l := range m.Render(w, h) {
		b.WriteString(sidebar.Plain(l) + "\n")
	}
	return b.String()
}

// TestName: what is typed shows as the name it makes and its file; Enter
// chooses that name.
func TestName(t *testing.T) {
	testutil.Isolate(t)
	i18n.SetLocale("en")
	m := New(func(string) bool { return false })
	if s := screen(m, 60, 18); !strings.Contains(s, "New environment") || !strings.Contains(s, "e.g. glm") {
		t.Fatalf("empty dialog:\n%s", s)
	}
	// In the popup (78×22 with its border): what an environment can hold.
	if s := screen(m, 76, 20); !strings.Contains(s, "For example:") || !strings.Contains(s, "• a proxy") || !strings.Contains(s, "• Claude Code on another endpoint") {
		t.Fatalf("its uses:\n%s", s)
	}
	typeText(m, "GLM Coding")
	if s := screen(m, 60, 18); !strings.Contains(s, "→ ") || !strings.Contains(s, "/env/glm-coding.env") {
		t.Fatalf("the name it makes:\n%s", s)
	}
	if name, done := m.Key("enter"); !done || name != "glm-coding" {
		t.Fatalf("enter: %q %v", name, done)
	}
}

// TestRefuses: Enter with nothing typed, or on a name an environment has,
// says so and stays; Esc closes with no name.
func TestRefuses(t *testing.T) {
	i18n.SetLocale("en")
	m := New(func(n string) bool { return n == "glm" })
	if _, done := m.Key("enter"); done || !strings.Contains(screen(m, 60, 18), "⚠ Type a name for the environment") {
		t.Fatalf("empty name:\n%s", screen(m, 60, 18))
	}
	typeText(m, "GLM")
	if s := screen(m, 60, 18); !strings.Contains(s, "⚠ An environment named glm already exists") {
		t.Fatalf("taken:\n%s", s)
	}
	if _, done := m.Key("enter"); done {
		t.Fatal("a taken name is not chosen")
	}
	typeText(m, "2")
	if name, done := m.Key("enter"); !done || name != "glm2" {
		t.Fatalf("enter: %q %v", name, done)
	}
	if name, done := New(nil).Key("esc"); !done || name != "" {
		t.Fatalf("esc: %q %v", name, done)
	}
}

// TestEditing: the cursor keys, deleting, a word, a paste and clearing.
func TestEditing(t *testing.T) {
	m := New(nil)
	typeText(m, "proxy")
	m.Key("home")
	typeText(m, "work ")
	m.Key("end")
	m.Key("backspace")
	m.Key("left")
	m.Key("left")
	m.Key("delete")
	if got := string(m.r); got != "work prx" || m.Name() != "work-prx" {
		t.Fatalf("got %q (%s)", got, m.Name())
	}
	m.Key("ctrl-w")
	if got := string(m.r); got != "work x" || m.cur != 5 {
		t.Fatalf("ctrl-w: %q at %d", got, m.cur)
	}
	m.Paste("a\tb\nc\x07")
	if got := string(m.r); got != "work a b cx" || m.Name() != "work-a-b-cx" {
		t.Fatalf("paste: %q", got)
	}
	m.Key("ctrl-u")
	if len(m.r) != 0 || m.cur != 0 {
		t.Fatal("ctrl-u clears")
	}
}

// TestRenderFits: the dialog fills its popup, no line wider than it, the
// cursor in sight however long the text.
func TestRenderFits(t *testing.T) {
	i18n.SetLocale("en")
	m := New(nil)
	typeText(m, strings.Repeat("abcdefghij", 10)+"END")
	for _, size := range [][2]int{{20, 8}, {40, 12}, {78, 22}, {10, 3}} {
		w, h := max(size[0], 20), max(size[1], 8)
		lines := m.Render(size[0], size[1])
		if len(lines) != h {
			t.Fatalf("%v: %d lines", size, len(lines))
		}
		for _, l := range lines {
			if lw := sidebar.DisplayWidth(sidebar.Plain(l)); lw > w {
				t.Fatalf("%v: %d columns: %q", size, lw, sidebar.Plain(l))
			}
		}
		if s := screen(m, size[0], size[1]); !strings.Contains(s, "END") {
			t.Fatalf("%v: the cursor's end of the text is not shown:\n%s", size, s)
		}
	}
}
