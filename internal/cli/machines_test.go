package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/machine"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
	"github.com/hamzayogurtcuoglu/i-mux/internal/watch"
)

func machineState(states ...string) machine.State {
	p := machine.Project{Session: "web", Name: "web"}
	for i, s := range states {
		p.Panes = append(p.Panes, machine.Pane{ID: "%" + string(rune('1'+i)), Agent: "claude", State: s})
	}
	return machine.State{Status: machine.StatusOK, Projects: []machine.Project{p}}
}

// TestRemoteNotices (T78.3): a machine's agent that turns to Needs you, or
// stops after working long enough, is notified; the first report and a
// short turn are not.
func TestRemoteNotices(t *testing.T) {
	nt := &remoteNotices{working: map[string]time.Time{}}
	t0 := time.Unix(1_800_000_000, 0)
	if n := nt.compare("box", machine.State{}, machineState("blocked", "working"), false, t0); len(n) != 0 {
		t.Fatalf("first report notified %+v", n)
	}
	// %2 worked since t0; %1 still blocked: nothing new.
	if n := nt.compare("box", machineState("blocked", "working"), machineState("blocked", "working"), true, t0.Add(time.Second)); len(n) != 0 {
		t.Fatalf("no change notified %+v", n)
	}
	n := nt.compare("box", machineState("blocked", "working"), machineState("idle", "idle"), true, t0.Add(watch.MinWork+time.Second))
	if len(n) != 1 || !strings.Contains(n[0].title, "finished") || n[0].project != "web" {
		t.Fatalf("finished %+v", n)
	}
	// A short turn: no notice; then a question.
	nt.compare("box", machineState("idle", "idle"), machineState("working", "idle"), true, t0.Add(time.Minute))
	if n := nt.compare("box", machineState("working", "idle"), machineState("idle", "idle"), true, t0.Add(time.Minute+time.Second)); len(n) != 0 {
		t.Fatalf("short turn notified %+v", n)
	}
	n = nt.compare("box", machineState("idle", "idle"), machineState("idle", "blocked"), true, t0.Add(2*time.Minute))
	if len(n) != 1 || !strings.Contains(n[0].title, "needs") && !strings.Contains(n[0].title, "Needs") {
		t.Fatalf("question %+v", n)
	}
}

func TestMachineStateChanged(t *testing.T) {
	a, b := machineState("idle"), machineState("idle")
	a.At, b.At = time.Unix(1, 0), time.Unix(2, 0)
	if changed(a, b) {
		t.Error("only the time changed")
	}
	if !changed(a, machineState("working")) {
		t.Error("a state changed")
	}
}

// TestRemoteArgsChecked (T78.2): what a machine was asked to attach to, or
// a machine's name, is checked before tmux or ssh sees it.
func TestRemoteArgsChecked(t *testing.T) {
	testutil.Isolate(t)
	for _, args := range [][]string{
		{"__remote-attach"},
		{"__remote-attach", "-t"},
		{"__remote-attach", "a;b"},
		{"__remote-attach", "ok", "1"},
		{"__remote-view", "box"},
		{"__remote-view", "box", "-x"},
	} {
		if _, _, code := runCLI(t, args...); code != ExitUsage {
			t.Errorf("%q: exit %d", args, code)
		}
	}
	t.Setenv("IMUX_SSH", "/nonexistent/ssh") // never reached
	for _, args := range [][]string{{"machine", "add", "-oProxyCommand=touch x"}, {"machine", "add", "--", "-oProxyCommand=touch x"}, {"machine", "add", "box", "ali@-oProxyCommand=x"}} {
		if _, _, code := runCLI(t, args...); code == ExitOK {
			t.Errorf("%q: accepted", args)
		}
	}
	if ms, _ := machine.Load(); len(ms) != 0 {
		t.Errorf("added %+v", ms)
	}
}
