package sidebar

import (
	"bytes"
	"context"
	"strconv"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
)

// longExplorer is a sidebar on an explorer of 50 files in 20 rows (the
// title and 19 rows of the list).
func longExplorer() *sidebar {
	m := model()
	m.View = ViewExplorer
	for i := range 50 {
		name := "f" + strconv.Itoa(100+i) + ".go"
		m.Tree = append(m.Tree, TreeRow{Path: name, Name: name})
	}
	var out bytes.Buffer
	s := &sidebar{o: Options{Out: &out}, model: m, win: Window{Width: 43, Height: 20}, sel: NoItem, noTip: NoItem}
	s.draw()
	return s
}

// selectedPath is the file the keyboard selection is on ("" for none).
func (s *sidebar) selectedPath() string {
	return s.screen.ActionOf(s.sel).Path
}

// TestWheelStopsAtTheEnd (T71.4): the wheel goes no further than the last
// page, so the first turn back up scrolls at once.
func TestWheelStopsAtTheEnd(t *testing.T) {
	i18n.SetLocale("en")
	s := longExplorer()
	l := s.screen.List
	if l.Start != 1 || l.N != 19 || l.Len != 50 || l.Top != 0 {
		t.Fatalf("list %+v", l)
	}
	ctx := context.Background()
	for range 40 {
		s.handle(ctx, Event{Kind: EvScroll, Key: "down", X: 20, Y: 5})
	}
	if s.scroll != 31 || s.screen.List.Top != 31 {
		t.Fatalf("scrolled to %d (drawn %d), want the last page 31", s.scroll, s.screen.List.Top)
	}
	s.handle(ctx, Event{Kind: EvScroll, Key: "up", X: 20, Y: 5})
	if s.screen.List.Top != 28 {
		t.Fatalf("one turn up: top %d, want 28", s.screen.List.Top)
	}
	// A model scrolled past the end (the list got shorter): up moves from
	// what is drawn.
	s.scroll, s.model.Scroll = 400, 400
	s.draw()
	s.handle(ctx, Event{Kind: EvScroll, Key: "up", X: 20, Y: 5})
	if s.screen.List.Top != 28 {
		t.Fatalf("up from past the end: top %d", s.screen.List.Top)
	}
	// Agents has no list: the wheel does nothing.
	s.model.View = ViewAgents
	s.draw()
	s.handle(ctx, Event{Kind: EvScroll, Key: "down", X: 20, Y: 5})
	if s.scroll != 0 {
		t.Fatalf("agents scrolled to %d", s.scroll)
	}
}

// TestKeysScrollTheList (T71.4): ↓ at the last row shown scrolls the list a
// row and selects the next file, down to the last one; ↑ comes back up the
// same way; past the end the selection goes around the screen.
func TestKeysScrollTheList(t *testing.T) {
	i18n.SetLocale("en")
	s := longExplorer()
	// Onto the list: down until a file is selected.
	for range 20 {
		if s.move(1); s.selectedPath() != "" {
			break
		}
	}
	if s.selectedPath() != "f100.go" {
		t.Fatalf("first file selected: %q", s.selectedPath())
	}
	for i := 1; i < 50; i++ {
		s.move(1)
		if want := "f" + strconv.Itoa(100+i) + ".go"; s.selectedPath() != want {
			t.Fatalf("down %d: %q, want %q (top %d)", i, s.selectedPath(), want, s.screen.List.Top)
		}
		if !s.screen.List.Holds(s.sel.Row) {
			t.Fatalf("down %d: the selection is off the list's rows", i)
		}
	}
	if s.screen.List.Top != 31 {
		t.Fatalf("at the last file the top is %d", s.screen.List.Top)
	}
	for i := 48; i >= 0; i-- {
		s.move(-1)
		if want := "f" + strconv.Itoa(100+i) + ".go"; s.selectedPath() != want {
			t.Fatalf("up to %d: %q, want %q", i, s.selectedPath(), want)
		}
	}
	if s.screen.List.Top != 0 {
		t.Fatalf("back at the first file the top is %d", s.screen.List.Top)
	}
	// Up from the first file leaves the list (the title, the rail).
	s.move(-1)
	if s.selectedPath() != "" && s.screen.List.Holds(s.sel.Row) && s.sel.Cell != 0 {
		t.Fatalf("up from the first file stayed on the list: %q", s.selectedPath())
	}
}
