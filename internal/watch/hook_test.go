package watch

import (
	"testing"
	"time"
)

// TestHookOverride (T76.3): a hook decides blocked and idle at once,
// working while the busy line shows or for a moment after it; the busy
// line wins.
func TestHookOverride(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	quiet := now.Add(-WorkingFor)
	for _, c := range []struct {
		name         string
		state        string
		hookAgo      time.Duration
		changedAgo   time.Duration
		asking, busy bool
		wantWorking  bool
		wantAsking   bool
	}{
		{"working, just said", "working", time.Second, time.Minute, false, false, true, false},
		{"working, busy line", "working", time.Minute, time.Minute, false, true, true, false},
		{"working, interrupted (no hook, no busy line)", "working", time.Minute, time.Minute, false, false, false, false},
		{"blocked at once", "blocked", 0, 0, false, false, false, true},
		{"blocked, granted (busy)", "blocked", time.Second, 0, false, true, true, false},
		{"idle at once, though the screen just redrew", "idle", 0, 0, false, false, false, false},
		{"idle, a new turn on screen (Codex)", "idle", time.Minute, 0, false, true, true, false},
	} {
		changed, asking := hookOverride(c.state, now.Add(-c.hookAgo), now, now.Add(-c.changedAgo), c.asking, c.busy)
		working := changed.After(quiet)
		if working != c.wantWorking || asking != c.wantAsking {
			t.Errorf("%s: working %v asking %v, want %v %v", c.name, working, asking, c.wantWorking, c.wantAsking)
		}
	}
}

// TestHooked: a hook from before the run is ignored; once the busy line
// showed after a blocked hook, the screen decides again (a pause between
// two tools is no new question).
func TestHooked(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	d := &Detector{Now: func() time.Time { return now }, changed: map[string]time.Time{}, busy: map[string]bool{}}
	start := now.Add(-time.Hour)
	p := PaneRow{ID: "%1", Start: start, Hook: "blocked", HookAt: start.Add(-time.Minute)}
	d.changed["%1"] = now
	d.hooked(p)
	if d.asking["%1"] {
		t.Error("a hook from before the run")
	}
	p.HookAt = now.Add(-10 * time.Second)
	d.hooked(p)
	if !d.asking["%1"] {
		t.Fatal("blocked: no question")
	}
	// Granted: the busy line shows, the screen changed after the hook.
	d.busy["%1"], d.changed["%1"], d.asking["%1"] = true, now, false
	d.hooked(p)
	// Between two tools: no busy line, a quiet screen; the spent hook
	// does not ask again.
	d.busy["%1"], d.changed["%1"] = false, now.Add(-time.Minute)
	d.hooked(p)
	if d.asking["%1"] {
		t.Error("the spent blocked hook asked again")
	}
	// A new hook counts again.
	p.HookAt = now
	d.hooked(p)
	if !d.asking["%1"] {
		t.Error("a new blocked hook ignored")
	}
}
