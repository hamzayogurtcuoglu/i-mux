package watch

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// The Approver answers one prompt per pane, once, and counts them on the
// pane (D112); a prompt that stays is left to the user, never tried again.
func TestApprover(t *testing.T) {
	clk := testutil.Clock()
	f := &fakeServer{screens: map[string]string{}, screensE: map[string]string{}}
	a := &Approver{T: &tmux.Client{Bin: "tmux", Socket: "t", Runner: f}, On: true}
	ctx := context.Background()
	start := clk.Now()
	inst := func(id string) string { return id + "@" + strconv.FormatInt(start.UnixMilli(), 10) }
	// row is a pane seen by a tick, approved what its option says.
	row := func(id string, approved int) PaneRow {
		return PaneRow{ID: id, Agent: session.Claude, Start: start, Approved: approved}
	}
	prompt := "Do you want to proceed?\n❯ 1. Yes\n  2. No\n"
	commands := func() string {
		if len(f.execs) == 0 {
			return ""
		}
		args := f.execs[len(f.execs)-1]
		if len(args) > 3 {
			args = args[3:]
		}
		return strings.Join(args, " ")
	}

	// One prompt: one key and the count, in one exec.
	f.screens["%1"] = prompt
	a.Tick(ctx, []PaneRow{row("%1", 0)}, f.screens, clk.Now())
	if got, want := commands(), "send-keys -t %1 1 ; set-option -p -q -t %1 @imux_approved 1"; got != want {
		t.Fatalf("first tick:\n%s\nwant\n%s", got, want)
	}
	if st := a.answered[inst("%1")]; st == nil || st.count != 1 {
		t.Fatalf("state after the answer: %+v", a.answered[inst("%1")])
	}

	// The same prompt again: nothing is sent, nothing counted.
	n := len(f.execs)
	a.Tick(ctx, []PaneRow{row("%1", 1)}, f.screens, clk.Now())
	if len(f.execs) != n {
		t.Fatalf("the same prompt answered again: %s", commands())
	}

	// A prompt still on screen long after its key: the user's, for good.
	clk.Set(clk.Now().Add(DefaultGiveUp + time.Second))
	a.Tick(ctx, []PaneRow{row("%1", 1)}, f.screens, clk.Now())
	if st := a.answered[inst("%1")]; !st.failed {
		t.Fatal("a prompt that stayed was not given up")
	}
	clk.Set(clk.Now().Add(time.Minute))
	a.Tick(ctx, []PaneRow{row("%1", 1)}, f.screens, clk.Now())
	if len(f.execs) != n {
		t.Fatalf("a given-up prompt answered again: %s", commands())
	}

	// The prompt is gone, then it comes back identical: it is a new one.
	f.screens["%1"] = "Working…\n"
	a.Tick(ctx, []PaneRow{row("%1", 1)}, f.screens, clk.Now())
	f.screens["%1"] = prompt
	a.Tick(ctx, []PaneRow{row("%1", 1)}, f.screens, clk.Now())
	if got, want := commands(), "send-keys -t %1 1 ; set-option -p -q -t %1 @imux_approved 2"; got != want {
		t.Fatalf("the new prompt:\n%s\nwant\n%s", got, want)
	}

	// The pane was looked at (its count was reset): the next answer counts
	// from one again.
	f.screens["%1"] = "Working…\n"
	a.Tick(ctx, []PaneRow{row("%1", 0)}, f.screens, clk.Now())
	f.screens["%1"] = prompt
	a.Tick(ctx, []PaneRow{row("%1", 0)}, f.screens, clk.Now())
	if got, want := commands(), "send-keys -t %1 1 ; set-option -p -q -t %1 @imux_approved 1"; got != want {
		t.Fatalf("after a look:\n%s\nwant\n%s", got, want)
	}

	// Two more panes, two prompts: one exec answers both.
	f.screens["%2"] = prompt
	f.screens["%3"] = prompt
	a.Tick(ctx, []PaneRow{row("%1", 1), row("%2", 0), row("%3", 0)}, f.screens, clk.Now())
	if got := commands(); !strings.Contains(got, "send-keys -t %2 1") || !strings.Contains(got, "send-keys -t %3 1") {
		t.Fatalf("two panes in one exec: %s", got)
	}

	// Off: nothing is ever sent.
	f.execs = nil
	off := &Approver{T: a.T, On: false}
	off.Tick(ctx, []PaneRow{row("%1", 1)}, f.screens, clk.Now())
	if len(f.execs) != 0 {
		t.Fatal("an off Approver answered")
	}

	// A pane that is gone is forgotten.
	a.Tick(ctx, []PaneRow{row("%9", 0)}, map[string]string{"%9": prompt}, clk.Now())
	if _, ok := a.answered[inst("%9")]; !ok {
		t.Fatal("a pane that was never seen")
	}
	a.Tick(ctx, []PaneRow{row("%1", 3)}, f.screens, clk.Now())
	if _, ok := a.answered[inst("%9")]; ok {
		t.Fatal("a gone pane is not forgotten")
	}
}

// OpenCode's buttons are answered with Enter only when a capture with the
// screen's colors shows "Allow once" highlighted (D112): one engine query
// for the colors, then the key.
func TestApproverOpenCode(t *testing.T) {
	clk := testutil.Clock()
	f := &fakeServer{screens: map[string]string{}, screensE: map[string]string{}}
	a := &Approver{T: &tmux.Client{Bin: "tmux", Socket: "t", Runner: f}, On: true}
	ctx := context.Background()
	prompt := "△ Permission required\n Allow once  Allow always  Reject  enter confirm\n"
	f.screens["%4"] = prompt
	oc := PaneRow{ID: "%4", Agent: session.OpenCode, Start: clk.Now()}
	tick := func() {
		a.Tick(ctx, []PaneRow{oc}, f.screens, clk.Now())
	}
	commands := func() string {
		if len(f.execs) == 0 {
			return ""
		}
		args := f.execs[len(f.execs)-1]
		if len(args) > 3 {
			args = args[3:]
		}
		return strings.Join(args, " ")
	}

	// The colors say "Allow once" is highlighted: Enter.
	f.screensE["%4"] = "\x1b[48;5;71m Allow once \x1b[48;5;24m Allow always \x1b[48;5;24m Reject \x1b[49m"
	tick()
	if got, want := commands(), "send-keys -t %4 Enter ; set-option -p -q -t %4 @imux_approved 1"; got != want {
		t.Fatalf("highlighted:\n%s\nwant\n%s", got, want)
	}

	// A new prompt whose colors say otherwise: only the colors are read.
	clk.Set(clk.Now().Add(time.Minute))
	f.screens["%4"] = "△ Permission required\n  bash command: rm -rf build\n Allow once  Allow always  Reject  enter confirm\n"
	f.screensE["%4"] = "\x1b[48;5;24m Allow once \x1b[48;5;71m Allow always \x1b[48;5;24m Reject \x1b[49m"
	n := len(f.execs)
	tick()
	if len(f.execs) != n+1 || !strings.Contains(commands(), "capture-pane -p -e -t %4") {
		t.Fatalf("the colors were not read: %s", commands())
	}
	for _, e := range f.execs[n:] {
		if strings.Contains(strings.Join(e, " "), "send-keys -t %4") {
			t.Fatalf("another button highlighted, yet answered: %s", strings.Join(e, " "))
		}
	}
}
