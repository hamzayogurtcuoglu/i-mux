//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

const sidebarConfig = "[tmux]\nsidebar = true\nagent_bar = false\n"

// sidebarPanes starts claude with the sidebar and returns the agent pane and
// the sidebar pane.
func (h *H) startWithSidebar() (agentPane, bar string) {
	h.t.Helper()
	h.mustImux("claude", "-d")
	h.waitFor("sidebar pane", 10*time.Second, func() bool {
		agentPane, bar = "", ""
		for _, l := range strings.Split(strings.TrimSpace(h.mustTmux("list-panes", "-a", "-F", "#{pane_id} #{@imux_role} #{@imux_agent}")), "\n") {
			f := strings.Fields(l)
			switch {
			case len(f) >= 2 && f[1] == "sidebar":
				bar = f[0]
			case len(f) >= 2:
				agentPane = f[0]
			}
		}
		return agentPane != "" && bar != ""
	})
	return agentPane, bar
}

// screen captures pane row by row (no -J: the sidebar's full-width rows
// would be joined).
func (h *H) screen(pane string) string {
	out, _ := h.tmux("capture-pane", "-p", "-t", pane)
	return out
}

// lineOf returns the sidebar line containing text.
func (h *H) lineOf(bar, text string) string {
	for _, l := range strings.Split(h.screen(bar), "\n") {
		if strings.Contains(l, text) {
			return l
		}
	}
	return ""
}

// plusOf returns the client screen position of the "+" of a section
// header ("PROJECTS", "AGENTS").
func (h *H) plusOf(bar, title string) (x, y int) {
	h.t.Helper()
	_, y = h.rowOf(bar, " "+title+" ")
	left, _ := strconv.Atoi(strings.TrimSpace(h.mustTmux("display-message", "-p", "-t", bar, "#{pane_left}")))
	w, _ := strconv.Atoi(strings.TrimSpace(h.mustTmux("display-message", "-p", "-t", bar, "#{pane_width}")))
	return left + w - 1, y
}

// rowOf returns the client screen position (1-based) of the first sidebar
// line containing text.
func (h *H) rowOf(bar, text string) (x, y int) {
	h.t.Helper()
	// The sidebar takes its width back after a client attached.
	h.waitFor("sidebar width", 10*time.Second, func() bool {
		f := strings.Fields(h.mustTmux("display-message", "-p", "-t", bar, "#{pane_width} #{window_width}"))
		ww, _ := strconv.Atoi(f[1])
		return f[0] == "40" || ww < 54
	})
	width, _ := strconv.Atoi(strings.TrimSpace(h.mustTmux("display-message", "-p", "-t", bar, "#{pane_width}")))
	var lines []string
	h.waitFor("sidebar line "+text, 10*time.Second, func() bool {
		lines = strings.Split(h.screen(bar), "\n")
		// Drawn for this width: the "+" of Projects is in the last but one
		// column (a capture right after a resize still shows the old frame).
		if len(lines) == 0 || sidebar.DisplayWidth(strings.TrimRight(lines[0], " ")) != width-1 {
			return false
		}
		for _, l := range lines {
			if strings.Contains(l, text) {
				return true
			}
		}
		return false
	})
	pos := strings.Fields(h.mustTmux("display-message", "-p", "-t", bar, "#{pane_left} #{pane_top} #{status-position} #{status}"))
	left, _ := strconv.Atoi(pos[0])
	top, _ := strconv.Atoi(pos[1])
	if pos[2] == "top" && pos[3] != "off" {
		top++ // the tab bar takes the client's first line
	}
	for i, l := range lines {
		if j := strings.Index(l, text); j >= 0 {
			return left + sidebar.DisplayWidth(l[:j]) + 1, top + i + 1
		}
	}
	return 0, 0
}

// TestSidebar (T10.6): the sidebar shows the window's panes and the agents to
// continue in; a real click on "Antigravity" switches the agent pane with
// context; "Split right" asks for an agent and opens a pane with it; the
// sidebar never becomes a target; prefix b hides and shows it; closing the
// last agent pane closes the window.
func TestSidebar(t *testing.T) {
	h := newH(t, sidebarConfig)
	agentPane, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	// A bigger client grows the panes; the sidebar takes its width back.
	h.waitFor("sidebar width 40", 5*time.Second, func() bool {
		return strings.TrimSpace(h.mustTmux("display-message", "-p", "-t", bar, "#{pane_width}")) == "40"
	})
	if got := h.paneOpt(agentPane, "#{pane_active}"); got != "1" {
		t.Errorf("the agent pane should keep the focus, active=%s", got)
	}

	// A click on "Antigravity" under "Continue this chat in" switches the
	// agent pane.
	x, y := h.rowOf(bar, "▲ Antigravity")
	c.Click(0, x, y)
	argv := h.run("antigravity", 1)
	if len(argv) != 2 || argv[0] != "-i" || !strings.HasPrefix(argv[1], "[i-mux handoff · Claude Code → Antigravity CLI") ||
		!strings.Contains(argv[1], "fake claude request 1") {
		t.Fatalf("antigravity argv %.300q", argv)
	}
	h.waitFor("agent pane runs antigravity", 5*time.Second, func() bool { return h.paneOpt(agentPane, "#{@imux_agent}") == "antigravity" })
	h.waitFor("sidebar shows the antigravity card", 5*time.Second, func() bool {
		for _, l := range strings.Split(h.capture(bar), "\n") {
			if strings.Contains(l, "▲  Antigravity") {
				return true
			}
		}
		return false
	})
	if h.paneOpt(bar, "#{@imux_agent}") != "" {
		t.Fatal("the sidebar must never run an agent")
	}

	// The "+" of Agents asks which agent, then splits the wide agent pane side
	// by side. A person does not click twice within tmux's double-click time.
	time.Sleep(500 * time.Millisecond)
	x, y = h.plusOf(bar, "AGENTS")
	mark := c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "Split right: new pane with", 5*time.Second)
	c.Write("x")
	h.run("codex", 1)
	var panes string
	h.waitFor("codex pane next to antigravity", 5*time.Second, func() bool {
		panes = strings.Join(strings.Fields(h.mustTmux("list-panes", "-t", agentPane, "-F", "#{@imux_role}:#{@imux_agent}")), " ")
		return panes == "sidebar: :antigravity :codex"
	})
	h.rowOf(bar, "◎  Codex") // its row

	// prefix b hides the sidebar, and shows it again.
	c.Write(keyPrefix + "b")
	h.waitFor("sidebar hidden", 5*time.Second, func() bool {
		return !strings.Contains(h.mustTmux("list-panes", "-t", agentPane, "-F", "#{@imux_role}"), "sidebar")
	})
	c.Write(keyPrefix + "b")
	h.waitFor("sidebar shown", 5*time.Second, func() bool {
		return strings.Contains(h.mustTmux("list-panes", "-t", agentPane, "-F", "#{@imux_role}"), "sidebar")
	})

	// Closing every agent pane closes the window: the sidebar leaves with it.
	win := strings.TrimSpace(h.mustTmux("display-message", "-p", "-t", agentPane, "#{window_id}"))
	for _, l := range strings.Split(strings.TrimSpace(h.mustTmux("list-panes", "-t", agentPane, "-F", "#{pane_id} #{@imux_role}")), "\n") {
		if f := strings.Fields(l); len(f) == 1 {
			h.mustTmux("kill-pane", "-t", f[0])
		}
	}
	h.waitFor("window closed", 10*time.Second, func() bool {
		out, _ := h.tmux("list-windows", "-a", "-F", "#{window_id}")
		return !strings.Contains(out, win)
	})
}

// TestSidebarDoubleClick: people double-click buttons. A double click on
// the "+" of Agents opens the menu once, the menu stays open (tmux's late
// DoubleClick1Pane must not close it) and the key typed next goes to the
// menu, not to the sidebar, where "x" would switch the agent pane instead.
func TestSidebarDoubleClick(t *testing.T) {
	h := newH(t, sidebarConfig)
	agentPane, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	x, y := h.plusOf(bar, "AGENTS")
	mark := c.Mark()
	c.Click(0, x, y)
	time.Sleep(80 * time.Millisecond)
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "Split right: new pane with", 5*time.Second)
	time.Sleep(600 * time.Millisecond) // past the double-click window
	c.Write("x")
	h.run("codex", 1)
	h.waitFor("codex in a new pane", 5*time.Second, func() bool {
		return strings.Contains(h.mustTmux("list-panes", "-t", agentPane, "-F", "#{@imux_agent}"), "codex")
	})
	if got := h.paneOpt(agentPane, "#{@imux_agent}"); got != "claude" {
		t.Fatalf("the key switched the agent pane to %q instead of splitting", got)
	}
	if n := strings.Count(h.mustTmux("list-panes", "-t", agentPane, "-F", "#{@imux_agent}"), "codex"); n != 1 || h.runs("codex") != 1 {
		t.Fatalf("split ran %d times (%d codex runs)", n, h.runs("codex"))
	}
}

// TestSidebarProjectsNamesChanges (T11.6): the sidebar's CHANGES row opens
// the file's diff, "Name this pane" names the pane (quotes are safe), and
// the Projects + opens another folder as a project and switches to it;
// clicking the first project in PROJECTS switches back.
func TestSidebarProjectsNamesChanges(t *testing.T) {
	h := newH(t, sidebarConfig)
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
		cmd.Dir = h.Homes.Project
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	testutil.WriteFile(t, filepath.Join(h.Homes.Project, "readme.md"), []byte("hello\n"))
	git("init", "-q", "-b", "main")
	git("add", "-A")
	git("commit", "-q", "-m", "init")
	testutil.WriteFile(t, filepath.Join(h.Homes.Project, "readme.md"), []byte("hello\nchanged line for the diff\n"))

	agentPane, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, client := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)

	// Source Control (⑂ on the rail) lists the changed file with its line
	// counts; a click shows the whole file with the change marked.
	x, y := h.rowOf(bar, "🔀")
	c.Click(0, x, y)
	x, y = h.rowOf(bar, "M  readme.md")
	if l := h.lineOf(bar, "M  readme.md"); !strings.Contains(l, "+1") {
		t.Errorf("source control row %q", l)
	}
	mark := c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "+ changed line for the diff", 5*time.Second)
	c.waitScreenAfter(h, mark, "1   hello", 5*time.Second) // the unchanged line too
	c.Write("q")                                           // less
	time.Sleep(500 * time.Millisecond)
	x, y = h.rowOf(bar, "🤖")
	c.Click(0, x, y) // back to the Agents view
	h.rowOf(bar, "AGENTS")
	time.Sleep(500 * time.Millisecond)

	// Name this pane: the prompt's answer becomes the name, quotes and all.
	x, y = h.rowOf(bar, "Rename")
	mark = c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "Name for this pane:", 5*time.Second)
	c.Write(`backend "fix"; true` + "\r")
	h.waitFor("pane named", 5*time.Second, func() bool { return h.paneOpt(agentPane, "#{@imux_name}") == `backend "fix"; true` })
	h.rowOf(bar, `backend "fix"`)
	if b := h.mustTmux("display-message", "-p", "-t", agentPane, "#{T:pane-border-format}"); !strings.Contains(b, `backend "fix"; true · Claude`) {
		t.Errorf("border %q", b)
	}

	// Open a project (the folder picker): a new session for the folder, and the
	// client moves there.
	other := filepath.Join(h.Homes.Home, "blog")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	x, y = h.plusOf(bar, "PROJECTS")
	mark = c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "Open a project", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	c.Write("~/blo") // a typed path is browsed: ~/blog is listed
	c.waitScreenAfter(h, mark, "▸ ~/blog", 5*time.Second)
	c.Write("\r")
	h.run("claude", 2)
	session := func() string {
		for _, l := range strings.Split(h.mustTmux("list-clients", "-F", "#{client_name} #{client_session}"), "\n") {
			if f := strings.Fields(l); len(f) == 2 && f[0] == client {
				return f[1]
			}
		}
		return ""
	}
	first := strings.TrimSpace(h.mustTmux("display-message", "-p", "-t", agentPane, "#{session_name}"))
	h.waitFor("client in the new project", 5*time.Second, func() bool { s := session(); return s != "" && s != first })
	if !strings.HasPrefix(session(), "blog-") {
		t.Fatalf("client session %q", session())
	}
	// The new project's sidebar lists the first project; a click goes back.
	var newBar string
	for _, l := range strings.Split(h.mustTmux("list-panes", "-s", "-t", session(), "-F", "#{pane_id} #{@imux_role}"), "\n") {
		if f := strings.Fields(l); len(f) == 2 && f[1] == "sidebar" {
			newBar = f[0]
		}
	}
	if newBar == "" {
		t.Fatal("the opened project has no sidebar")
	}
	time.Sleep(500 * time.Millisecond)
	x, y = h.rowOf(newBar, " "+filepath.Base(h.Homes.Project)+" ")
	c.Click(0, x, y)
	h.waitFor("client back in the first project", 5*time.Second, func() bool { return session() == first })
}

// TestSidebarNeedsYou (T13.1): an agent that asks a question on a still
// screen shows "Needs you" in the sidebar, on its row and its project.
func TestSidebarNeedsYou(t *testing.T) {
	h := newH(t, sidebarConfig)
	h.control("claude", "printf ' Do you want to proceed?\\n ❯ 1. Yes\\n   2. No\\n'\n")
	_, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	h.waitFor("Needs you on the agent and the project", 15*time.Second, func() bool {
		return strings.Count(h.capture(bar), "Needs you") == 2
	})
}

// TestSidebarFold (T19.1): a click on a section title folds it in every
// sidebar (@imux_fold) and a second click unfolds it.
func TestSidebarFold(t *testing.T) {
	h := newH(t, sidebarConfig)
	_, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	x, y := h.rowOf(bar, "▾ PROJECTS")
	c.Click(0, x+2, y)
	h.waitFor("projects folded", 5*time.Second, func() bool {
		return strings.TrimSpace(h.mustTmux("show-options", "-gqv", "@imux_fold")) == "projects" && h.lineOf(bar, "▸ PROJECTS") != ""
	})
	time.Sleep(500 * time.Millisecond)
	x, y = h.rowOf(bar, "▸ PROJECTS")
	c.Click(0, x+2, y)
	h.waitFor("projects unfolded", 5*time.Second, func() bool {
		return strings.TrimSpace(h.mustTmux("show-options", "-gqv", "@imux_fold")) == "" && h.lineOf(bar, "▾ PROJECTS") != ""
	})
}

// TestClaudeStatusline (T19.3): with claude_statusline Claude starts with
// --settings= naming imux's status line.
func TestClaudeStatusline(t *testing.T) {
	h := newH(t, "[limits]\nclaude_statusline = true\npoll_interval = \"200ms\"\n")
	h.startSession("claude")
	argv := h.run("claude", 1)
	if len(argv) < 1 || !strings.HasPrefix(argv[0], "--settings=") {
		t.Fatalf("claude argv %q", argv)
	}
	if b := h.readFile(strings.TrimPrefix(argv[0], "--settings=")); !strings.Contains(b, "__statusline claude") {
		t.Fatalf("settings %s", b)
	}
}
