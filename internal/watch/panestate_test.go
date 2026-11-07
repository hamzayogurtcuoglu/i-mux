package watch

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// A pane is working while its screen changed in the last 3 s, blocked when
// its still screen asks a question; a state is written only when it changes,
// with its time, and the end of work is recorded for "Done".
func TestPaneStates(t *testing.T) {
	s := &PaneStates{}
	now := testutil.FrozenTime
	ts := func(t time.Time) string { return strconv.FormatInt(t.Unix(), 10) }
	join := func(cmds [][]string) string {
		var out []string
		for _, c := range cmds {
			out = append(out, strings.Join(c, " "))
		}
		return strings.Join(out, "; ")
	}
	cmds := func(at time.Time, changed map[string]time.Time, asking map[string]bool) string {
		var out [][]string
		for _, c := range s.Commands(at, changed, asking) {
			out = append(out, c)
		}
		return join(out)
	}
	got := cmds(now, map[string]time.Time{"%1": now.Add(-time.Second), "%2": now.Add(-time.Minute)}, nil)
	want := "set-option -p -q -t %1 @imux_state working; set-option -p -q -t %1 @imux_state_at " + ts(now) +
		"; set-option -p -q -t %2 @imux_state idle; set-option -p -q -t %2 @imux_state_at " + ts(now)
	if got != want {
		t.Fatalf("first tick\n%s\nwant\n%s", got, want)
	}
	if got := cmds(now.Add(time.Second), map[string]time.Time{"%1": now.Add(-time.Second), "%2": now.Add(-time.Minute)}, nil); got != "" {
		t.Fatalf("unchanged states written again: %v", got)
	}
	// Work ends: idle, and the end is recorded.
	at := now.Add(5 * time.Second)
	got = cmds(at, map[string]time.Time{"%1": now.Add(-time.Second)}, nil)
	if got != "set-option -p -q -t %1 @imux_state idle; set-option -p -q -t %1 @imux_state_at "+ts(at)+"; set-option -p -q -t %1 @imux_done_at "+ts(at) {
		t.Fatalf("working → idle %v", got)
	}
	if _, ok := s.last["%2"]; ok {
		t.Fatal("a gone pane is forgotten")
	}
	// A question on a still screen: blocked; while it changes: working.
	if got := cmds(at, map[string]time.Time{"%1": now.Add(-time.Second)}, map[string]bool{"%1": true}); !strings.Contains(got, "@imux_state blocked") || strings.Contains(got, "done_at") {
		t.Fatalf("idle → blocked %v", got)
	}
	if got := cmds(at, map[string]time.Time{"%1": at}, map[string]bool{"%1": true}); !strings.Contains(got, "@imux_state working") {
		t.Fatalf("asking while changing %v", got)
	}
}

// Synthetic screens of each CLI's questions, and plain output that is not
// one.
func TestAsking(t *testing.T) {
	for _, screen := range []string{
		"● Bash(rm -rf build)\n\n Do you want to proceed?\n ❯ 1. Yes\n   2. No, and tell Claude what to do differently (esc)\n",
		" Do you want to make this edit to main.go?\n",
		"Would you like to run the following command?\n  $ npm test\n› 1. Yes, proceed (y)\n",
		"Overwrite config? (y/N)",
		"Continue? [Y/n] ",
		" Allow this action?\n",
		"  1. Yes\n  2. Yes, and don't ask again\n  3. No\n",
	} {
		if !Asking(screen) {
			t.Errorf("not seen as a question:\n%s", screen)
		}
	}
	for _, screen := range []string{
		"",
		"I updated the tests. Do you want me to also update the docs?\n" + strings.Repeat("output line\n", 13),
		"Tests: 14 passed\n> \n",
		"The function allows this behavior when the flag is set.\n",
		"Step 1. Yesterday we changed the parser.\n",
		"? for shortcuts\n",
	} {
		if Asking(screen) {
			t.Errorf("seen as a question:\n%s", screen)
		}
	}
}
