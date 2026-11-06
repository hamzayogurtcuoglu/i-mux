package watch

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/clock"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/limits"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// fakeServer answers the detector's two execs.
type fakeServer struct {
	clients string            // list-clients rows
	panes   string            // list-panes rows
	screens map[string]string // pane → capture
	execs   [][]string
}

func (f *fakeServer) Run(_ context.Context, _ string, args []string) ([]byte, []byte, error) {
	f.execs = append(f.execs, args)
	var b strings.Builder
	for i := 0; i < len(args); i++ {
		if args[i] == "display-message" && i+2 < len(args) && strings.HasPrefix(args[i+2], tmux.Delim) {
			b.WriteString(args[i+2] + "\n")
		}
		switch args[i] {
		case "list-clients":
			b.WriteString(f.clients)
		case "list-panes":
			b.WriteString(f.panes)
		case "capture-pane":
			b.WriteString(f.screens[args[i+4]])
		}
	}
	return []byte(b.String()), nil, nil
}

func (f *fakeServer) captures() int {
	n := 0
	for _, e := range f.execs {
		for _, a := range e {
			if a == "capture-pane" {
				n++
			}
		}
	}
	return n
}

type harness struct {
	f      *fakeServer
	d      *Detector
	clk    *clock.Fixed
	events []Event
	act    bool // OnEvent's return value (prompted)
}

func newHarness(t *testing.T) *harness {
	cfg := config.Default()
	set, err := limits.Compile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{f: &fakeServer{screens: map[string]string{}}, clk: testutil.Clock(), act: true}
	h.d = &Detector{
		T:        &tmux.Client{Bin: "tmux", Socket: "t", Runner: h.f},
		Patterns: set, Cfg: cfg.Limits, Now: h.clk.Now,
		OnEvent: func(_ context.Context, ev Event, _ []Client) bool {
			h.events = append(h.events, ev)
			return h.act && ev.Severity == limits.SevLimit
		},
	}
	return h
}

func (h *harness) tick(t *testing.T) (attached bool, execs int) {
	t.Helper()
	before := len(h.f.execs)
	a, err := h.d.Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return a, len(h.f.execs) - before
}

func ms(t time.Time) string { return strconv.FormatInt(t.UnixMilli(), 10) }

func TestDetectorExecBudget(t *testing.T) {
	h := newHarness(t)
	start := ms(testutil.FrozenTime.Add(-time.Hour))
	h.f.panes = "%1\tclaude\t" + start + "\t0\t40\ts\n%2\t\t\t0\t40\ts\n%3\tgemini\t" + start + "\t1\t40\ts\n%4\tcodex\t" + start + "\t0\t10\ts\n"
	// No client attached: one exec, no captures.
	attached, n := h.tick(t)
	if attached || n != 1 || h.f.captures() != 0 {
		t.Fatalf("detached: attached=%v execs=%d captures=%d", attached, n, h.f.captures())
	}
	// A control-mode client does not count.
	h.f.clients = "ctl\t1\ts\t1\n"
	if attached, _ := h.tick(t); attached {
		t.Fatal("control client counted as attached")
	}
	h.f.clients += "/dev/pts/1\t0\ts\t2\n"
	attached, n = h.tick(t)
	if !attached || n != 2 {
		t.Fatalf("attached: %v, %d execs (want ≤ 2)", attached, n)
	}
	last := strings.Join(h.f.execs[len(h.f.execs)-1], " ")
	// Live agent panes only (not the shell %2, not the dead %3), bottom
	// scan_lines of the screen.
	if !strings.Contains(last, "capture-pane -p -J -t %1") || !strings.Contains(last, "capture-pane -p -J -t %4") ||
		strings.Contains(last, "%2") || strings.Contains(last, "%3") {
		t.Fatalf("capture batch: %s", last)
	}
}

func TestDetectorGuards(t *testing.T) {
	h := newHarness(t)
	now := testutil.FrozenTime
	h.f.clients = "/dev/pts/1\t0\tproj\t1\n"
	h.f.panes = "%1\tclaude\t" + ms(now.Add(-time.Hour)) + "\t0\t30\tproj\n"
	limitLine := "│ 5-hour limit reached ∙ resets 3pm │"
	h.f.screens["%1"] = "working...\n" + limitLine + "\n"
	h.tick(t)
	if len(h.events) != 1 || h.events[0].Severity != limits.SevLimit || h.events[0].Text != "5-hour limit reached ∙ resets 3pm" ||
		h.events[0].Pane != "%1" || h.events[0].Session != "proj" || h.events[0].PatternID == "" {
		t.Fatalf("first event %+v", h.events)
	}
	// Same screen: skipped by hash. Redrawn screen with the same line: seen.
	h.tick(t)
	h.f.screens["%1"] = "spinner ⠋\n" + limitLine + "\n"
	h.tick(t)
	if len(h.events) != 1 {
		t.Fatalf("repeat line prompted again: %+v", h.events)
	}
	// A different limit line during the cooldown: no prompt; a warn line
	// still reaches the status bar.
	h.clk.Set(now.Add(5 * time.Minute))
	h.f.screens["%1"] = "Weekly limit reached ∙ resets Nov 8, 9am\nApproaching weekly limit\n"
	h.tick(t)
	if len(h.events) != 2 || h.events[1].Severity != limits.SevWarn {
		t.Fatalf("cooldown: %+v", h.events)
	}
	// After the cooldown a new limit line prompts again.
	h.clk.Set(now.Add(11 * time.Minute))
	h.f.screens["%1"] = "Opus weekly limit reached ∙ resets 3pm\n"
	h.tick(t)
	if len(h.events) != 3 || h.events[2].Severity != limits.SevLimit {
		t.Fatalf("after cooldown: %+v", h.events)
	}
	// The ignore pattern never fires.
	h.f.screens["%1"] = "Claude Opus limit reached, now using Sonnet 4\n"
	h.clk.Set(now.Add(30 * time.Minute))
	h.tick(t)
	if len(h.events) != 3 {
		t.Fatalf("ignore pattern fired: %+v", h.events[3:])
	}
}

func TestDetectorGraceAndNewRun(t *testing.T) {
	h := newHarness(t)
	now := testutil.FrozenTime
	h.f.clients = "/dev/pts/1\t0\tproj\t1\n"
	// A resumed session re-renders an old limit message 5 s after start.
	h.f.panes = "%1\tclaude\t" + ms(now.Add(-5*time.Second)) + "\t0\t30\tproj\n"
	line := "5-hour limit reached ∙ resets 3pm"
	h.f.screens["%1"] = line + "\n"
	h.tick(t)
	h.clk.Set(now.Add(time.Minute)) // grace over, line still on screen
	h.f.screens["%1"] = line + "\nmore output\n"
	h.tick(t)
	if len(h.events) != 0 {
		t.Fatalf("message within grace prompted: %+v", h.events)
	}
	// A new run in the same pane (new @ajan_start) has a fresh seen-set.
	h.f.panes = "%1\tgemini\t" + ms(now.Add(-time.Minute)) + "\t0\t30\tproj\n"
	h.f.screens["%1"] = "Usage limit reached for gemini-2.5-pro.\n"
	h.act = false
	h.tick(t)
	if len(h.events) != 1 || h.events[0].Agent != "gemini" {
		t.Fatalf("new run: %+v", h.events)
	}
	// The pane closes: its state is dropped.
	h.f.panes = ""
	h.tick(t)
	if len(h.d.hashes) != 0 || len(h.d.seen) != 0 {
		t.Fatalf("state kept for gone panes: %v %v", h.d.hashes, h.d.seen)
	}
}

func TestLastLines(t *testing.T) {
	if got := strings.Join(lastLines("a\nb\nc\nd\n\n   \n\n", 2), ","); got != "c,d" {
		t.Fatalf("got %q", got)
	}
	if got := strings.Join(lastLines("x\n", 25), ","); got != "x" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"\x1b[31m│ 5-hour limit reached ∙ resets 3pm │\x1b[0m": "5-hour limit reached ∙ resets 3pm",
		"  ⎿  You've hit your usage limit.   ":                 "You've hit your usage limit.",
		"> fix   the  bug":                                     "fix the bug",
		"────────":                                             "",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}
