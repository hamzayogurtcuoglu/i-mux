package workspace

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func TestPaneMenuDisplayMenuArgs(t *testing.T) {
	w, f, _ := testWS(t)
	now := testutil.FrozenTime
	reset := time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, time.Local)
	if reset.Before(now) {
		reset = now.Add(30 * time.Minute)
	}
	w.st = &state.State{Limits: map[string]*state.Limit{
		"gemini": {At: now, ResetAt: reset, ExpiresAt: reset, Severity: "limit"},
		"claude": {At: now.Add(-6 * time.Hour), ExpiresAt: now.Add(-time.Hour)}, // expired
	}}
	p := &Pane{ID: "%3", Agent: "claude", CWD: "/home/test/proj", Trail: Trail{{Agent: "claude"}}}
	if err := w.Show(context.Background(), w.PaneMenu(p, "/dev/pts/4"), "/dev/pts/4", "%3"); err != nil {
		t.Fatal(err)
	}
	args := f.calls[len(f.calls)-1]
	if args[2] != "display-menu" {
		t.Fatalf("not a display-menu: %q", args)
	}
	got := strings.Join(args, "\x00")
	until := reset.Local().Format("15:04")
	wantSeq := []string{
		"-c\x00/dev/pts/4\x00-t\x00%3\x00-T\x00ajan · Claude\x00-x\x00C\x00-y\x00C",
		"Switch to Gemini (with context) (limited until " + until + ")\x00g\x00run-shell -b '/opt/ajan/bin/ajan switch gemini --pane %3 --client /dev/pts/4 >/dev/null 2>&1 || true'",
		"-Switch to Copilot (with context) (not installed)\x00p\x00",
		"-Switch to Codex (with context) (not installed)\x00x\x00",
		"Sessions…\x00s\x00run-shell -b '/opt/ajan/bin/ajan __menu --sessions --pane %3 --client /dev/pts/4",
		"New pane → Claude\x00C\x00run-shell -b '/opt/ajan/bin/ajan split claude -h --pane %3",
		"New pane → Gemini\x00G\x00",
		"Preview handoff\x00v\x00display-popup -E -w 90% -h 90% -c /dev/pts/4 -d /home/test/proj '/opt/ajan/bin/ajan handoff --pane %3 --print 2>&1 | less -R'",
		"Limits\x00l\x00display-message -c /dev/pts/4 'The limits panel is coming soon'",
		"Close pane\x00k\x00kill-pane -t %3",
	}
	pos := 0
	for _, w := range wantSeq {
		i := strings.Index(got[pos:], w)
		if i < 0 {
			t.Fatalf("display-menu args lack (in order) %q\nargs: %q", w, args)
		}
		pos += i + len(w)
	}
	for _, bad := range []string{"Switch to Claude", "New pane → Codex", "Claude: limited"} {
		if strings.Contains(got, bad) {
			t.Errorf("unexpected %q", bad)
		}
	}
	// Separators are single empty arguments, never doubled or at the edges.
	for i := 1; i < len(args); i++ {
		if args[i] == "" && args[i-1] == "" {
			t.Fatalf("double separator at %d: %q", i, args)
		}
	}
	if args[len(args)-1] == "" {
		t.Fatal("trailing separator")
	}
}

func TestPaneMenuLimitedCurrentAndShell(t *testing.T) {
	w, _, _ := testWS(t)
	now := testutil.FrozenTime
	w.st = &state.State{Limits: map[string]*state.Limit{"claude": {At: now, ExpiresAt: now.Add(time.Hour)}}}
	m := w.PaneMenu(&Pane{ID: "%1", Agent: "claude"}, "")
	if !m.Items[0].Disabled || !strings.HasPrefix(m.Items[0].Name, "Claude: limited until ") {
		t.Fatalf("first item %+v", m.Items[0])
	}
	// A plain shell pane offers to start agents; preview needs a trail.
	m = w.PaneMenu(&Pane{ID: "%2"}, "")
	if m.Title != "ajan · shell" || !strings.HasPrefix(m.Items[0].Name, "Start Claude here") {
		t.Fatalf("shell menu %+v", m)
	}
	for _, it := range m.Items {
		if it.Name == "Preview handoff" && !it.Disabled {
			t.Fatal("preview should be disabled without a trail")
		}
	}
	// Names are escaped for tmux formats.
	cmd := (&Menu{Title: "a#b", Items: []MenuItem{{Name: "x#{y}", Key: "1", Cmd: "c"}}}).Cmd("", "")
	if strings.Join(cmd, " ") != "display-menu -T a##b -x C -y C x##{y} 1 c" {
		t.Fatalf("escaping: %q", cmd)
	}
}
