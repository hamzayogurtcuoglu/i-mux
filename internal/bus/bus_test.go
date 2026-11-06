package bus

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/clock"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

// fakeTmux answers list-panes / display-message and serves screens that can
// change between captures.
type fakeTmux struct {
	panes   string            // list-panes rows (workspace pane format)
	screens map[string]string // pane → screen
	onCap   func()            // after each capture exec
	calls   [][]string
}

func (f *fakeTmux) Run(_ context.Context, _ string, args []string) ([]byte, []byte, error) {
	f.calls = append(f.calls, args)
	var b strings.Builder
	captured := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "list-panes":
			b.WriteString(f.panes)
		case "display-message":
			if i+2 < len(args) && strings.HasPrefix(args[i+2], tmux.Delim) {
				b.WriteString(strings.ReplaceAll(args[i+2], "%%", "%") + "\n") // as tmux expands it
			}
		case "capture-pane":
			b.WriteString(f.screens[args[i+4]])
			captured = true
		}
	}
	if captured && f.onCap != nil {
		f.onCap()
	}
	return []byte(b.String()), nil, nil
}

// row renders one pane in the workspace's list format.
func row(id, agent, session string, dead bool, start time.Time) string {
	d := "0"
	if dead {
		d = "1"
	}
	ms := ""
	if !start.IsZero() {
		ms = itoa(start.UnixMilli())
	}
	return strings.Join([]string{id, agent, ms, "/home/test/proj", "/home/test/proj", agent, d, session, "@1", "node", "", "", "[]"}, "\t") + "\n"
}

func itoa(n int64) string {
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if len(b) == 0 {
		return "0"
	}
	return string(b)
}

func newBus(t *testing.T, f *fakeTmux) (*Bus, *clock.Fixed) {
	t.Helper()
	testutil.Isolate(t)
	clk := testutil.Clock()
	w := &workspace.WS{Cfg: config.Default(), T: &tmux.Client{Bin: "tmux", Socket: "t", Runner: f}, Self: "/opt/ajan", Now: clk.Now}
	b := New(w)
	b.Sleep = func(time.Duration) {}
	return b, clk
}

func TestResolve(t *testing.T) {
	now := testutil.FrozenTime
	f := &fakeTmux{panes: row("%1", "claude", "proj", false, now) + row("%2", "gemini", "proj", false, now) +
		row("%3", "gemini", "other", false, now) + row("%4", "codex", "other", false, now) + row("%5", "codex", "other", false, now) +
		row("%6", "copilot", "proj", true, now)}
	b, _ := newBus(t, f)
	ctx := context.Background()
	cases := []struct {
		target, caller, want string
		err                  error
	}{
		{"%3", "%1", "%3", nil},
		{"%9", "%1", "", ErrNoTarget},
		{"gemini", "%1", "%2", nil}, // the caller's session first
		{"gemini", "%4", "%3", nil},
		{"claude", "%4", "%1", nil}, // none locally: server-wide
		{"codex", "%1", "", ErrAmbiguous},
		{"copilot", "%1", "", ErrNoTarget}, // exited panes do not count
		{"vim", "%1", "", ErrNoTarget},
	}
	for _, c := range cases {
		p, err := b.Resolve(ctx, c.target, c.caller)
		switch {
		case c.err != nil && !errors.Is(err, c.err):
			t.Errorf("%s from %s: err %v, want %v", c.target, c.caller, err, c.err)
		case c.err == nil && (err != nil || p.ID != c.want):
			t.Errorf("%s from %s: got %v %v, want %s", c.target, c.caller, p, err, c.want)
		}
	}
	_, err := b.Resolve(ctx, "codex", "%1")
	if !strings.Contains(err.Error(), "%4 (other), %5 (other)") {
		t.Fatalf("candidates not listed: %v", err)
	}
}

func TestPanesBySampling(t *testing.T) {
	now := testutil.FrozenTime
	f := &fakeTmux{
		panes:   row("%1", "claude", "proj", false, now) + row("%2", "gemini", "proj", false, now) + row("%3", "", "proj", true, time.Time{}),
		screens: map[string]string{"%1": "spinner ⠋\n", "%2": "done.\n"},
	}
	f.onCap = func() { f.screens["%1"] += "⠙\n" } // %1 keeps redrawing
	b, _ := newBus(t, f)
	ps, err := b.Panes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, p := range ps {
		got[p.Pane] = p.State
	}
	if got["%1"] != "busy" || got["%2"] != "idle" || got["%3"] != "exited" {
		t.Fatalf("states %v", got)
	}
}

func TestPanesFromWatcherActivity(t *testing.T) {
	now := testutil.FrozenTime
	f := &fakeTmux{panes: row("%1", "claude", "proj", false, now.Add(-time.Hour)) + row("%2", "gemini", "proj", false, now.Add(-time.Hour))}
	b, _ := newBus(t, f)
	if err := WriteActivity(&Activity{At: now.UnixMilli(), Changed: map[string]int64{"%1": now.Add(-time.Second).UnixMilli(), "%2": now.Add(-42 * time.Second).UnixMilli()}}); err != nil {
		t.Fatal(err)
	}
	ps, err := b.Panes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ps[0].State != "busy" || ps[0].IdleSeconds != 1 || ps[1].State != "idle" || ps[1].IdleSeconds != 42 {
		t.Fatalf("panes %+v", ps)
	}
	for _, c := range f.calls {
		for _, a := range c {
			if a == "capture-pane" {
				t.Fatal("fresh watcher activity should avoid sampling")
			}
		}
	}
	// Stale activity is ignored.
	if ReadActivity(now.Add(time.Minute), 6*time.Second) != nil {
		t.Fatal("stale activity used")
	}
}
