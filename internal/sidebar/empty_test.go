package sidebar

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/gitinfo"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// The "No agents" pane lists the agents to start, centered, a key and a
// click on each; missing ones are listed but cannot start.
func TestRenderEmpty(t *testing.T) {
	i18n.SetLocale("en")
	agents := []AgentRow{
		{ID: "claude", Name: "Claude", Key: "c", Installed: true},
		{ID: "copilot", Name: "Copilot", Key: "p"},
		{ID: "codex", Name: "Codex", Key: "x", Installed: true},
	}
	sc := RenderEmpty("acme-api", agents, 80, 24, 2, "")
	text := strings.Join(sc.Lines, "\n")
	t.Logf("\n%s", Plain(text))
	for _, want := range []string{"No agents in acme-api", "Start one here:", "c  Claude", "p  Copilot", "(not installed)", "x  Codex", "t  Terminal (shell)", "or + next to AGENTS in the sidebar"} {
		if !strings.Contains(Plain(text), want) {
			t.Errorf("lacks %q", want)
		}
	}
	if len(sc.Lines) != len(sc.Item) || len(sc.Lines) > 24 {
		t.Fatalf("%d lines, %d items", len(sc.Lines), len(sc.Item))
	}
	for i, l := range sc.Lines {
		if n := DisplayWidth(Plain(l)); n > 80 {
			t.Errorf("line %d is %d columns", i, n)
		}
	}
	// A click on a row starts its item; Codex (selected) is highlighted.
	row := func(name string) int {
		for i, l := range sc.Lines {
			if strings.Contains(Plain(l), name) {
				return i
			}
		}
		t.Fatalf("no row %q", name)
		return -1
	}
	if it := sc.Item[row("Claude")]; it != 0 {
		t.Errorf("Claude's row starts item %d", it)
	}
	if it := sc.Item[row("Terminal")]; sc.items[it].id != ShellItem {
		t.Errorf("Terminal's row starts %+v", sc.items[it])
	}
	if it := sc.Item[row("No agents in")]; it != -1 {
		t.Error("the title starts something")
	}
	if !strings.Contains(sc.Lines[row("Codex")], selBg) {
		t.Error("the selection is not highlighted")
	}
	// Keys: an installed agent's letter, never a missing one's; ↑↓ skip it.
	if sc.pick("c") != 0 || sc.pick("p") != -1 || sc.pick("t") != 3 || sc.pick("z") != -1 {
		t.Errorf("pick c=%d p=%d t=%d", sc.pick("c"), sc.pick("p"), sc.pick("t"))
	}
	if sc.move(-1, 1) != 0 || sc.move(0, 1) != 2 || sc.move(2, -1) != 0 || sc.move(-1, -1) != 3 || sc.move(3, 1) != 0 {
		t.Errorf("move: %d %d %d %d %d", sc.move(-1, 1), sc.move(0, 1), sc.move(2, -1), sc.move(-1, -1), sc.move(3, 1))
	}
	// Starting shows it in place of the hint.
	if s := Plain(strings.Join(RenderEmpty("acme-api", agents, 80, 24, 0, "Claude").Lines, "\n")); !strings.Contains(s, "Starting Claude…") || strings.Contains(s, "next to AGENTS") {
		t.Errorf("starting:\n%s", s)
	}
	// A small pane cuts rows, never wraps them.
	for _, l := range RenderEmpty("a-very-long-project-name-indeed", agents, 20, 5, -1, "").Lines {
		if n := DisplayWidth(Plain(l)); n > 20 {
			t.Errorf("small pane: %d columns: %q", n, Plain(l))
		}
	}
}

// A project whose last agent closed keeps its sidebar: AGENTS says so and
// starts one, and nothing offers to continue a chat.
func TestNoAgentsSidebar(t *testing.T) {
	i18n.SetLocale("en")
	m := model()
	m.Projects[0].Panes = nil
	m.Panes, m.NoAgents = nil, true
	sc := Render(m, 43, 40, NoItem)
	text := screenText(sc)
	t.Logf("\n%s", text)
	lines := strings.Split(text, "\n")
	under := false
	for i := 1; i < len(lines); i++ {
		if strings.HasSuffix(lines[i-1], "│ acme-api") && strings.HasSuffix(lines[i], "│   No agents · + to start one") {
			under = true
		}
	}
	if !under {
		t.Fatalf("no agents row under the project:\n%s", text)
	}
	if strings.Contains(text, "Continue this chat in") {
		t.Error("continuing offered without a chat")
	}
	for _, l := range sc.Lines {
		if strings.Contains(Plain(l.Text), "No agents") {
			if a := l.ActionAt(20); a.Kind != ActNewAgent {
				t.Errorf("the row does %+v", a)
			}
		}
	}
}

// TestGatherNoAgents: a window with the sidebar and the "No agents" pane is
// not alone; the pane is the target but no agent; the project stays listed.
func TestGatherNoAgents(t *testing.T) {
	h := testutil.Isolate(t)
	i18n.SetLocale("en")
	c := realTmux(t)
	ctx := context.Background()
	out, err := c.Run(ctx, tmux.Cmd{"new-session", "-d", "-x", "120", "-y", "40", "-c", h.Project, "-P", "-F", "#{pane_id}", "sh"})
	if err != nil {
		t.Fatal(err)
	}
	empty := strings.TrimSpace(out)
	out, _ = c.Run(ctx, tmux.Cmd{"split-window", "-h", "-b", "-f", "-d", "-l", "30", "-t", empty, "-P", "-F", "#{pane_id}", "sh"})
	bar := strings.TrimSpace(out)
	cmds := append([]tmux.Cmd{{"set-option", "-p", "-t", bar, OptRole, RoleSidebar}}, EmptyOpts(empty, h.Project)...)
	if _, err := c.Run(ctx, cmds...); err != nil {
		t.Fatal(err)
	}
	src := &Source{
		T: c, Cfg: config.Default(), Now: time.Now,
		Git: func(context.Context, string) gitinfo.Info { return gitinfo.Info{} },
	}
	m, win, err := src.Gather(ctx, bar)
	if err != nil {
		t.Fatal(err)
	}
	if win.Alone || win.Windows != 1 || m.Target != empty || !m.NoAgents || len(m.Panes) != 0 {
		t.Fatalf("window %+v target %q noAgents %v panes %+v", win, m.Target, m.NoAgents, m.Panes)
	}
	if len(m.Projects) != 1 || !m.Projects[0].Current || len(m.Projects[0].Panes) != 0 || m.Project != "proj" {
		t.Fatalf("projects %+v project %q", m.Projects, m.Project)
	}
	for _, a := range m.Agents {
		if a.Current {
			t.Errorf("%s is current in a pane without agents", a.ID)
		}
	}
	if got, _ := c.Run(ctx, tmux.Cmd{"show-options", "-wqv", "-t", bar, OptTabAuto}); strings.TrimSpace(got) != "No agents" {
		t.Errorf("tab name %q", got)
	}
}
