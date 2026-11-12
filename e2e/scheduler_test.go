//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/auto"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// buttonOf returns the client position of glyph on the first sidebar line
// with text.
func (h *H) buttonOf(bar, text, glyph string) (x, y int) {
	h.t.Helper()
	_, y = h.rowOf(bar, text)
	l := h.lineOf(bar, text)
	j := strings.Index(l, glyph)
	if j < 0 {
		h.t.Fatalf("no %s on the line %q", glyph, l)
	}
	left, _ := strconv.Atoi(strings.TrimSpace(h.mustTmux("display-message", "-p", "-t", bar, "#{pane_left}")))
	return left + sidebar.DisplayWidth(l[:j]) + 1, y
}

// headless is the argv of agent's n-th print-mode run.
func (h *H) headless(agent string, n int) []string {
	h.t.Helper()
	p := filepath.Join(h.Log, agent+"-headless-"+itoa(n)+".argv")
	var argv []string
	h.waitFor(agent+" print-mode run "+itoa(n), 10*time.Second, func() bool {
		b, err := os.ReadFile(p)
		argv = splitNUL(b)
		return err == nil && len(argv) > 0
	})
	return argv
}

// TestScheduler (T66.4): the ⏰ view lists the project's agents and
// skills. 🚀 asks the CLI (the agent's own first) and the model the first
// time, runs the agent headless with --agent and shows its report; ⏰ →
// Every 5 minutes → Claude → Default schedules a skill, which starts at
// once; the watcher starts a schedule that is due; ⋯ shows a run's output
// and removes the schedule. ✨ New agent… makes a blank agent file or has
// this pane's agent write one.
func TestScheduler(t *testing.T) {
	ed := filepath.Join(t.TempDir(), "fake-editor")
	testutil.WriteFile(t, ed, []byte("#!/bin/sh\necho \"EDIT $*\"\nread x\n"))
	if err := os.Chmod(ed, 0o755); err != nil {
		t.Fatal(err)
	}
	h := newH(t, "editor = \""+ed+"\"\n"+sidebarConfig)
	t.Setenv("IMUX_AUTO_CHECK", "300ms")
	p := h.Homes.Project
	agentFile := filepath.Join(p, ".claude", "agents", "sim-tag.md")
	skillFile := filepath.Join(p, ".agents", "skills", "notes", "SKILL.md")
	testutil.WriteFile(t, agentFile, []byte("---\nname: sim-tag\ndescription: Watches the simulator's tags\n---\nCompare the newest tag.\n"))
	testutil.WriteFile(t, skillFile, []byte("---\nname: notes\ndescription: Writes release notes\n---\n# Notes\n"))
	reply := func(s string) { testutil.WriteFile(t, filepath.Join(h.Log, "claude.headless-reply"), []byte(s)) }
	testutil.WriteFile(t, filepath.Join(h.Log, "claude.headless-plain"), nil)
	schedules := func() string { b, _ := os.ReadFile(auto.Path()); return string(b) }

	_, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	h.openView(c, bar, "⏰", "schedule")
	h.rowOf(bar, "Watches the simulator's tags")
	h.rowOf(bar, "Writes release notes")
	h.rowOf(bar, "⏰ schedule · 🚀 run now")

	// 🚀 the first time: which CLI (Claude, the agent's own), which model
	// (the default); then it runs as Claude's own agent.
	reply("v2.3 is out: pipeline started\nThe simulator's pipeline runs.")
	time.Sleep(500 * time.Millisecond)
	x, y := h.buttonOf(bar, "sim-tag", "🚀")
	mark := c.Mark()
	c.Click(0, x+1, y)
	c.waitScreenAfter(h, mark, "Run sim-tag with", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	mark = c.Mark()
	c.Write("c")
	c.waitScreenAfter(h, mark, "Model for sim-tag · Claude", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	c.Write("\r")
	argv := h.headless("claude", 1)
	if len(argv) < 6 || argv[0] != "-p" || argv[1] != "--session-id" || argv[3] != "--agent" || argv[4] != "sim-tag" ||
		argv[5] != "--dangerously-skip-permissions" || strings.Contains(strings.Join(argv, " "), "--model") {
		t.Fatalf("argv %q", argv)
	}
	if prompt := argv[len(argv)-1]; !strings.Contains(prompt, "started by hand") || !strings.Contains(prompt, "This is the first run.") ||
		strings.Contains(prompt, "Compare the newest tag.") {
		t.Errorf("prompt:\n%s", prompt)
	}
	h.rowOf(bar, "v2.3 is out")
	h.rowOf(bar, "by hand · Claude")
	if s := schedules(); !strings.Contains(s, `name = "sim-tag"`) || !strings.Contains(s, `cli = "claude"`) || !strings.Contains(s, `when = ""`) {
		t.Errorf("schedules.toml:\n%s", s)
	}

	// ⋯ → Change time… → Custom… → a time typed in Turkish units.
	time.Sleep(500 * time.Millisecond)
	x, y = h.buttonOf(bar, "sim-tag", "⋯")
	mark = c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "Change time", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	mark = c.Mark()
	c.Write("t")
	c.waitScreenAfter(h, mark, "When should sim-tag run?", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	mark = c.Mark()
	c.Write("c")
	c.waitScreenAfter(h, mark, "When (10m", 5*time.Second)
	c.Write("2 saat\r")
	h.waitFor("every 2h", 5*time.Second, func() bool { return strings.Contains(schedules(), `when = "2h"`) })
	h.rowOf(bar, "every 2 h · Claude")

	// ⏰ on the skill: Every 5 minutes → Claude → Default. It is scheduled
	// and starts at once, with the skill's path in its prompt.
	reply("Release notes are written")
	time.Sleep(500 * time.Millisecond)
	x, y = h.buttonOf(bar, "notes", "⏰")
	mark = c.Mark()
	c.Click(0, x+1, y)
	c.waitScreenAfter(h, mark, "When should notes run?", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	mark = c.Mark()
	c.Write("5")
	c.waitScreenAfter(h, mark, "Run notes with", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	mark = c.Mark()
	c.Write("c")
	c.waitScreenAfter(h, mark, "Model for notes · Claude", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	c.Write("\r")
	argv = h.headless("claude", 2)
	if prompt := argv[len(argv)-1]; !strings.HasPrefix(prompt, `Use the Agent Skill "notes" at `+skillFile) || !strings.Contains(prompt, "on a schedule (every 5m)") {
		t.Errorf("skill prompt:\n%s", prompt)
	}
	if strings.Contains(strings.Join(argv, " "), "--agent") {
		t.Errorf("--agent for a skill: %q", argv)
	}
	h.rowOf(bar, "every 5 min · Claude")
	h.rowOf(bar, "Release notes are")
	if s := schedules(); !strings.Contains(s, `name = "notes"`) || !strings.Contains(s, `when = "5m"`) {
		t.Errorf("schedules.toml:\n%s", s)
	}

	// Due: its last run was 10 minutes ago. The watcher starts it.
	id := auto.ID(p, skillFile)
	old := time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)
	testutil.WriteFile(t, filepath.Join(h.Homes.Imux, "auto", id, "runs.json"), []byte(`[{"start":"`+old+`","end":"`+old+`","status":"ok"}]`))
	reply("Nothing new since " + old)
	argv = h.headless("claude", 3)
	if prompt := argv[len(argv)-1]; !strings.Contains(prompt, "on a schedule (every 5m)") {
		t.Errorf("watcher's run prompt:\n%s", prompt)
	}
	h.rowOf(bar, "Nothing new since")
	time.Sleep(time.Second)
	if _, err := os.Stat(filepath.Join(h.Log, "claude-headless-4.argv")); err == nil {
		t.Error("a schedule started twice")
	}

	// ⋯ → Last output: the run's log in a popup.
	time.Sleep(500 * time.Millisecond)
	x, y = h.buttonOf(bar, "notes", "⋯")
	mark = c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "Remove schedule", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	mark = c.Mark()
	c.Write("o")
	c.waitScreenAfter(h, mark, "fake-claude -p", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	c.Write("q")

	// A right-click → Remove schedule… → Remove: back among the skills.
	time.Sleep(700 * time.Millisecond)
	x, y = h.rowOf(bar, "notes")
	mark = c.Mark()
	c.Click(2, x, y)
	c.waitScreenAfter(h, mark, "Remove schedule", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	mark = c.Mark()
	c.Write("x")
	c.waitScreenAfter(h, mark, "Remove the schedule of notes?", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	c.Write("y")
	h.waitFor("the schedule removed", 5*time.Second, func() bool { return !strings.Contains(schedules(), `name = "notes"`) })
	h.rowOf(bar, "Writes release notes")
	h.rowOf(bar, "SCHEDULED  1")
	if _, err := os.Stat(filepath.Join(h.Homes.Imux, "auto", id)); !os.IsNotExist(err) {
		t.Errorf("its runs stayed: %v", err)
	}

	// ✨ New agent…: a blank agent file, named in the dialog and opened in
	// the editor; this pane's agent gets i-mux's agent-writing guide.
	newAgent := func() {
		t.Helper()
		time.Sleep(500 * time.Millisecond)
		x, y := h.buttonOf(bar, "SCHEDULER", "✨")
		mark := c.Mark()
		c.Click(0, x+1, y)
		c.waitScreenAfter(h, mark, "Who should write it?", 5*time.Second)
		time.Sleep(200 * time.Millisecond)
	}
	newAgent()
	mark = c.Mark()
	c.Write("t")
	c.waitScreenAfter(h, mark, "Agent name", 5*time.Second)
	mark = c.Mark()
	c.Write("Tag Watch")
	c.waitScreenAfter(h, mark, ".claude/agents/tag-watch.md", 5*time.Second)
	mark = c.Mark()
	c.Write("\r")
	c.waitScreenAfter(h, mark, "EDIT .claude/agents/tag-watch.md", 5*time.Second)
	c.Write("\r")
	h.rowOf(bar, "tag-watch")
	if b := h.readFile(filepath.Join(p, ".claude", "agents", "tag-watch.md")); !strings.HasPrefix(b, "---\nname: tag-watch\n") {
		t.Errorf("template:\n%s", b)
	}

	newAgent()
	mark = c.Mark()
	c.Write("h")
	c.waitScreenAfter(h, mark, "What should the agent do?", 5*time.Second)
	time.Sleep(200 * time.Millisecond)
	c.Write("watch the tags\r")
	h.waitFor("agent-writing guide sent to claude", 5*time.Second, func() bool {
		s := h.stdinOf("claude", 1)
		return strings.Contains(s, "Create an agent for this project") && strings.Contains(s, ".claude/agents/<name>.md") &&
			strings.Contains(s, "watch the tags")
	})
}
