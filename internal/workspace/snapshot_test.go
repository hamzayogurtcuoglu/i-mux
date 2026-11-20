package workspace

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// Capture reads tabs in order, skips sidebars, keeps names, layouts and each
// agent's current run.
func TestCapture(t *testing.T) {
	w, f, h := testWS(t)
	dir := h.Project
	row := func(win, tab, layout, active, role, agent, name, dead, trail string) string {
		return strings.Join([]string{dir, win, tab, layout, active, role, agent, name, dead, trail}, "\t")
	}
	f.replies = map[string]string{"list-panes": strings.Join([]string{
		row("0", "", "L0", "1", "sidebar", "", "", "0", ""),
		row("0", "", "L0", "1", "", "claude", "backend fix", "0", `[{"a":"codex","s":1},{"a":"claude","s":2,"id":"c-1"}]`),
		row("0", "", "L0", "1", "", "codex", "", "0", `[{"a":"codex","s":3}]`),
		row("1", "api work", "L1", "0", "sidebar", "", "", "0", ""),
		row("1", "api work", "L1", "0", "", "", "", "0", ""),          // a shell
		row("1", "api work", "L1", "0", "", "copilot", "", "1", `[]`), // dead
	}, "\n") + "\n"}
	got, err := w.Capture(context.Background())
	if err != nil || len(got) != 1 {
		t.Fatalf("capture %+v %v", got, err)
	}
	p := got[0]
	if p.Dir != dir || len(p.Tabs) != 2 || p.Agents() != 2 {
		t.Fatalf("project %+v", p)
	}
	t0, t1 := p.Tabs[0], p.Tabs[1]
	if !t0.Sidebar || !t0.Active || t0.Layout != "L0" || len(t0.Panes) != 2 || t0.Panes[0].Name != "backend fix" ||
		t0.Panes[0].Run == nil || t0.Panes[0].Run.SessionID != "c-1" || t0.Panes[1].Run == nil || t0.Panes[1].Run.Start != 3 {
		t.Fatalf("tab 0 %+v", t0)
	}
	if t1.Name != "api work" || t1.Active || len(t1.Panes) != 2 || t1.Panes[0].Agent != "" || t1.Panes[1].Agent != "" {
		t.Fatalf("tab 1 %+v", t1)
	}
}

// Merge: open projects replace their snapshots; one gone from this server
// was closed; one of an earlier server stays open (it died with it). The
// last session is every project of the earlier server, with or without
// agents (D75).
func TestMergeSnapshots(t *testing.T) {
	now := testutil.FrozenTime
	tab := []TabSnap{{Panes: []PaneSnap{{Agent: "claude"}}}}
	s := &Snapshots{Projects: []ProjectSnap{
		{Dir: "/a", Tabs: tab, Server: "s1", Open: true, At: now.Add(-time.Hour).Unix()},
		{Dir: "/b", Tabs: tab, Server: "s2", Open: true, At: now.Add(-time.Hour).Unix()},
		{Dir: "/c", Tabs: tab, Server: "s2", Open: true, At: now.Add(-2 * time.Hour).Unix()},
		{Dir: "/e", Tabs: []TabSnap{{Sidebar: true}}, Server: "s1", Open: true, At: now.Add(-3 * time.Hour).Unix()}, // "No agents"
	}}
	s.Merge([]ProjectSnap{{Dir: "/b", Tabs: append(tab, tab...)}, {Dir: "/d", Tabs: tab}}, "s2", now)
	get := func(d string) ProjectSnap { return *s.Find(d) }
	if a := get("/a"); !a.Open || a.Server != "s1" {
		t.Errorf("earlier server's project %+v", a)
	}
	if b := get("/b"); !b.Open || len(b.Tabs) != 2 || b.At != now.Unix() {
		t.Errorf("open project %+v", b)
	}
	if c := get("/c"); c.Open {
		t.Errorf("closed project %+v", c)
	}
	if s.Projects[0].At != now.Unix() || len(s.Projects) != 5 {
		t.Errorf("order %+v", s.Projects)
	}
	in := s.Interrupted("s2", map[string]bool{"/b": true})
	if len(in) != 2 || in[0].Dir != "/a" || in[1].Dir != "/e" {
		t.Errorf("interrupted %+v", in)
	}
	if in := s.Interrupted("s2", map[string]bool{"/a": true}); len(in) != 1 || in[0].Dir != "/e" {
		t.Errorf("an open project is not interrupted: %+v", in)
	}
}

// A project with no agent to start (only its "No agents" pane was open, or
// its agents are not installed) comes back with the "No agents" pane and
// its sidebar, starting no agent (D75); opening its folder on its own starts
// the default agent, as for a new project.
func TestRestoreWithoutAgents(t *testing.T) {
	testutil.SkipOnWindows(t, "POSIX agent paths (/opt/bin); Windows spawns agents in the e2e suite")
	w, f, h := testWS(t)
	ctx := context.Background()
	f.errs["has-session"] = "can't find session: x"
	f.replies["new-session"] = "%7\n"
	calls := func() []string {
		var out []string
		for _, c := range f.calls {
			out = append(out, strings.Join(c, " "))
		}
		f.calls = nil
		return out
	}
	// Codex is not installed (testWS).
	res, err := w.Restore(ctx, nil, &ProjectSnap{Dir: h.Project, Tabs: []TabSnap{{Sidebar: true, Panes: []PaneSnap{{Agent: "codex"}}}}})
	if err != nil || !res.Created || res.Pane != "%7" || res.Session != SessionName(h.Project) {
		t.Fatalf("restore %+v %v", res, err)
	}
	var batch, bar string
	for _, c := range calls() {
		if strings.Contains(c, "new-session") {
			batch = c
		}
		if strings.Contains(c, "-t %7") && strings.HasSuffix(c, "/opt/imux/bin/imux __sidebar") {
			bar = c
		}
	}
	for _, want := range []string{
		"new-session -d -s " + res.Session + " -c " + h.Project + " -P -F #{pane_id} /opt/imux/bin/imux __empty ;",
		"; set-option -p @imux_role empty", "; set-option -p @imux_cwd " + h.Project, "; set-option -p @imux_label No agents",
	} {
		if !strings.Contains(batch, want) {
			t.Errorf("batch lacks %q:\n%s", want, batch)
		}
	}
	if strings.Contains(batch, "__exec") {
		t.Errorf("an agent started: %s", batch)
	}
	if bar == "" {
		t.Error("no sidebar")
	}

	// No tab had a sidebar: none comes back.
	if _, err := w.Restore(ctx, nil, &ProjectSnap{Dir: h.Project, Tabs: []TabSnap{{}}}); err != nil {
		t.Fatal(err)
	}
	for _, c := range calls() {
		if strings.Contains(c, "__sidebar") {
			t.Errorf("a sidebar: %s", c)
		}
	}

	// Its folder opened on its own: the default agent, no "No agents".
	if err := (&Snapshots{Projects: []ProjectSnap{{Dir: h.Project, Tabs: []TabSnap{{Sidebar: true}}, Open: true}}}).Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Start(ctx, "", h.Project); err != nil {
		t.Fatal(err)
	}
	started := false
	for _, c := range calls() {
		started = started || strings.Contains(c, "new-session") && strings.Contains(c, "__exec")
		if strings.Contains(c, "__empty") {
			t.Errorf("the No agents pane: %s", c)
		}
	}
	if !started {
		t.Error("the default agent did not start")
	}
}
