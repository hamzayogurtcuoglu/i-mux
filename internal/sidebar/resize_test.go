package sidebar

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/gitops"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// A changed file shows its name whole and first, its folder after it in
// gray; a short row cuts the folder, then the middle of the name.
func TestFileLabel(t *testing.T) {
	for _, c := range []struct {
		path string
		w    int
		want string
	}{
		{"tests/_bdd/config.py", 20, "config.py tests/_bdd"},
		{"tests/_bdd/config.py", 18, "config.py tests/_…"},
		{"tests/_bdd/config.py", 30, "config.py tests/_bdd          "},
		{"automation-framework/tests/_bdd/steps/blim_steps.py", 26, "blim_steps.py automation-…"},
		{"a/b/blim_simulator.py", 12, "blim_s…or.py"},
		{"automation-framework/", 15, "automat…mework/"},
		{"automation-framework/", 25, "automation-framework/    "},
		{"tools/.env.example", 17, ".env.example too…"},
		{"src/x.go", 8, "x.go    "}, // no room for the folder: none of it
		{"notes.md", 10, "notes.md  "},
		{"MAC/", 10, "MAC/      "},
		{"x/y/", 10, "y/ x      "},    // an untracked folder in a folder
		{"日本語のファイル.md", 9, "日本 ….md"}, // a wide letter that does not fit: a space
	} {
		got := fileLabel(c.path, c.w)
		if p := Plain(got); p != c.want {
			t.Errorf("fileLabel(%q, %d) = %q, want %q", c.path, c.w, p, c.want)
		}
		if n := DisplayWidth(Plain(got)); n != c.w {
			t.Errorf("fileLabel(%q, %d) is %d columns", c.path, c.w, n)
		}
	}
	if got := fileLabel("tests/config.py", 25); !strings.Contains(got, muted+"tests") {
		t.Errorf("the folder is not gray: %q", got)
	}
}

// In the Source Control view a name keeps its start and extension at the
// default width, and a wider panel shows it whole with its folder.
func TestChangesViewNames(t *testing.T) {
	i18n.SetLocale("en")
	m := model()
	m.View = ViewChanges
	m.SCM = &gitops.Status{Branch: "main", Changes: []gitops.Change{
		{Status: "?", Path: "automation-framework/", Untracked: true},
		{Status: "M", Path: "automation-framework/tools/.env.example", Added: 4},
		{Status: "M", Path: "automation-framework/tests/_bdd/steps/blim_steps.py", Added: 47, Removed: 3},
		{Status: "M", Path: "automation-framework/src/collector/run_and_collect.py", Added: 65},
	}}
	for w, wants := range map[int][]string{
		43: {"?  automatio…ramework/", "M  .env.example     +4", "M  blim_s…ps.py +47 −3", "M  run_and…lect.py +65"},
		60: {
			"?  automation-framework/", "M  .env.example automation-framewor… +4",
			"M  blim_steps.py automation-fra… +47 −3", "M  run_and_collect.py automation-f… +65",
		},
	} {
		sc := Render(m, w, 0, NoItem)
		text := screenText(sc)
		t.Logf("\n%s", text)
		for _, want := range wants {
			if !strings.Contains(text, want) {
				t.Errorf("width %d lacks %q", w, want)
			}
		}
		for _, l := range sc.Lines {
			if n := DisplayWidth(Plain(l.Text)); n != w {
				t.Errorf("width %d: %d columns: %q", w, n, Plain(l.Text))
			}
		}
	}
}

func TestDragged(t *testing.T) {
	g := Geom{W: 43, H: 40, WinW: 160, Panes: "%0 %1 "}
	for _, c := range []struct {
		name      string
		prev, cur Geom
		want      bool
	}{
		{"edge dragged", g, Geom{W: 60, H: 40, WinW: 160, Panes: "%0 %1 "}, true},
		{"unchanged", g, g, false},
		{"first look", Geom{}, Geom{W: 60, WinW: 160, Panes: "%0 %1 "}, false},
		{"window resized", g, Geom{W: 58, H: 40, WinW: 200, Panes: "%0 %1 "}, false},
		{"pane closed next to it", Geom{W: 43, WinW: 160, Panes: "%0 %1 %2 "}, Geom{W: 100, WinW: 160, Panes: "%0 %2 "}, false},
	} {
		if got := Dragged(c.prev, c.cur); got != c.want {
			t.Errorf("%s: Dragged = %v", c.name, got)
		}
	}
	for in, want := range map[int]int{5: 20, 20: 20, 61: 61, 80: 80, 140: 80} {
		if got := ClampWidth(in); got != want {
			t.Errorf("ClampWidth(%d) = %d", in, got)
		}
	}
}

// TestSettleRealTmux: in a real tmux window, a sidebar takes a width the
// user dragged its edge to and saves it once; a resized window, a pane closed
// next to it and the rail of a closed panel keep the sidebar's width.
func TestSettleRealTmux(t *testing.T) {
	h := testutil.Isolate(t)
	c := realTmux(t)
	ctx := context.Background()
	saved := make(chan int, 8)
	oldSave, oldAfter := SaveWidth, SaveAfter
	SaveWidth = func(_ string, w int) error { saved <- w; return nil }
	SaveAfter = 20 * time.Millisecond
	t.Cleanup(func() { SaveWidth, SaveAfter = oldSave, oldAfter })

	out, err := c.Run(ctx, tmux.Cmd{"new-session", "-d", "-x", "120", "-y", "30", "-c", h.Project, "-P", "-F", "#{pane_id}", "sh"})
	if err != nil {
		t.Fatal(err)
	}
	agentPane := strings.TrimSpace(out)
	out, _ = c.Run(ctx, tmux.Cmd{"split-window", "-h", "-b", "-f", "-d", "-l", "30", "-t", agentPane, "-P", "-F", "#{pane_id}", "sh"})
	bar := strings.TrimSpace(out)
	width := func() int {
		out, _ := c.Run(ctx, tmux.Cmd{"display-message", "-p", "-t", bar, "#{pane_width}"})
		n, _ := strconv.Atoi(strings.TrimSpace(out))
		return n
	}
	cfg := config.Default()
	cfg.Tmux.SidebarWidth = 30
	src := &Source{T: c, Cfg: cfg, CfgPath: filepath.Join(h.Root, "config.toml")}
	s := &sidebar{o: Options{Source: src, Pane: bar, Width: 30}, t: c, save: make(chan struct{}, 1)}
	if s.keepWidth(ctx) || width() != 30 {
		t.Fatalf("first look: width %d", width())
	}

	// The user drags the edge (tmux's MouseDrag1Border runs resize-pane).
	_, _ = c.Run(ctx, tmux.Cmd{"resize-pane", "-t", bar, "-x", "45"})
	if !s.keepWidth(ctx) || s.o.Width != 45 || s.win.Width != 45 || width() != 45 {
		t.Fatalf("drag to 45: width %d, keeps %d, drawn %d", width(), s.o.Width, s.win.Width)
	}
	_, _ = c.Run(ctx, tmux.Cmd{"resize-pane", "-t", bar, "-x", "52"})
	if !s.keepWidth(ctx) || s.o.Width != 52 {
		t.Fatalf("drag on to 52: keeps %d", s.o.Width)
	}
	select {
	case <-s.save:
		s.saveWidth(ctx)
	case <-time.After(2 * time.Second):
		t.Fatal("the dragged width was never saved")
	}
	if w := <-saved; w != 52 || cfg.Tmux.SidebarWidth != 52 {
		t.Fatalf("saved %d, config %d", w, cfg.Tmux.SidebarWidth)
	}
	if len(saved) != 0 {
		t.Fatal("a drag is saved once, at its end")
	}

	// A bigger window grows every pane: the sidebar takes its width back.
	_, _ = c.Run(ctx, tmux.Cmd{"resize-window", "-t", bar, "-x", "180"})
	if width() == 52 {
		t.Fatal("tmux kept the sidebar's width; the test shows nothing")
	}
	if s.keepWidth(ctx) || width() != 52 {
		t.Fatalf("after the window grew: width %d", width())
	}

	// Closing the agent next to it gives the sidebar the room: not a drag.
	out, _ = c.Run(ctx, tmux.Cmd{"split-window", "-h", "-d", "-t", agentPane, "-P", "-F", "#{pane_id}", "sh"})
	if strings.TrimSpace(out) == "" {
		t.Fatal("no second agent pane")
	}
	s.keepWidth(ctx) // the hooks redraw after a split: the new pane is seen
	_, _ = c.Run(ctx, tmux.Cmd{"kill-pane", "-t", agentPane})
	if width() == 52 {
		t.Fatal("tmux gave the room elsewhere; the test shows nothing")
	}
	if s.keepWidth(ctx) || width() != 52 || s.o.Width != 52 {
		t.Fatalf("after closing the pane next to it: width %d, keeps %d", width(), s.o.Width)
	}

	// Narrower than the config allows: the narrowest allowed.
	_, _ = c.Run(ctx, tmux.Cmd{"resize-pane", "-t", bar, "-x", "12"})
	if !s.keepWidth(ctx) || s.o.Width != config.MinSidebarWidth || width() != config.MinSidebarWidth {
		t.Fatalf("drag to 12: width %d, keeps %d", width(), s.o.Width)
	}

	// The rail of a closed panel keeps its width.
	s.model.Closed = true
	_, _ = c.Run(ctx, tmux.Cmd{"resize-pane", "-t", bar, "-x", strconv.Itoa(ClosedWidth)})
	s.keepWidth(ctx)
	_, _ = c.Run(ctx, tmux.Cmd{"resize-pane", "-t", bar, "-x", "40"})
	if s.keepWidth(ctx) || width() != ClosedWidth || s.o.Width != config.MinSidebarWidth {
		t.Fatalf("closed panel dragged: width %d, keeps %d", width(), s.o.Width)
	}
}
