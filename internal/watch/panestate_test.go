package watch

import (
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// A pane is working while its screen changed in the last 3 s; a state is
// written only when it changes.
func TestPaneStates(t *testing.T) {
	s := &PaneStates{}
	now := testutil.FrozenTime
	join := func(cmds [][]string) string {
		var out []string
		for _, c := range cmds {
			out = append(out, strings.Join(c, " "))
		}
		return strings.Join(out, "; ")
	}
	got := s.Commands(now, map[string]time.Time{"%1": now.Add(-time.Second), "%2": now.Add(-time.Minute)})
	var cmds [][]string
	for _, c := range got {
		cmds = append(cmds, c)
	}
	if j := join(cmds); j != "set-option -p -q -t %1 @imux_state working; set-option -p -q -t %2 @imux_state idle" {
		t.Fatalf("first tick %q", j)
	}
	if got := s.Commands(now.Add(time.Second), map[string]time.Time{"%1": now.Add(-time.Second), "%2": now.Add(-time.Minute)}); len(got) != 0 {
		t.Fatalf("unchanged states written again: %v", got)
	}
	got = s.Commands(now.Add(5*time.Second), map[string]time.Time{"%1": now.Add(-time.Second)})
	if len(got) != 1 || strings.Join(got[0], " ") != "set-option -p -q -t %1 @imux_state idle" {
		t.Fatalf("working → idle %v", got)
	}
	if _, ok := s.last["%2"]; ok {
		t.Fatal("a gone pane is forgotten")
	}
}
