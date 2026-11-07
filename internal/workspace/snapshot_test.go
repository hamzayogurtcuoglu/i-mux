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
// was closed; one of an earlier server stays open (it died with it).
func TestMergeSnapshots(t *testing.T) {
	now := testutil.FrozenTime
	tab := []TabSnap{{Panes: []PaneSnap{{Agent: "claude"}}}}
	s := &Snapshots{Projects: []ProjectSnap{
		{Dir: "/a", Tabs: tab, Server: "s1", Open: true, At: now.Add(-time.Hour).Unix()},
		{Dir: "/b", Tabs: tab, Server: "s2", Open: true, At: now.Add(-time.Hour).Unix()},
		{Dir: "/c", Tabs: tab, Server: "s2", Open: true, At: now.Add(-2 * time.Hour).Unix()},
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
	if s.Projects[0].At != now.Unix() || len(s.Projects) != 4 {
		t.Errorf("order %+v", s.Projects)
	}
	in := s.Interrupted("s2", map[string]bool{"/b": true})
	if len(in) != 1 || in[0].Dir != "/a" {
		t.Errorf("interrupted %+v", in)
	}
	if len(s.Interrupted("s2", map[string]bool{"/a": true})) != 0 {
		t.Error("an open project is not interrupted")
	}
}
